package kube_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fieldsByPath indexes a diff by path for readable assertions.
func fieldsByPath(
	fields []inventory.FieldChange,
) map[string]inventory.FieldChange {
	out := map[string]inventory.FieldChange{}
	for _, f := range fields {
		out[f.Path] = f
	}
	return out
}

func editedDeployment(edit func(*corev1.Container)) (
	*appsv1.Deployment, *appsv1.Deployment,
) {
	before, after := deployment("api"), deployment("api")
	edit(&after.Spec.Template.Spec.Containers[0])
	return before, after
}

func TestDeploymentDiffReportsEnvValues(t *testing.T) {
	before, after := editedDeployment(func(c *corev1.Container) {
		c.Env = []corev1.EnvVar{{Name: "DB_HOST", Value: "db-new"}}
	})
	before.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "DB_HOST", Value: "db-old"}}

	got := fieldsByPath(kube.DeploymentSchema().Diff(before, after))

	field := got["containers[app].env.DB_HOST"]
	assert.Equal(t, "db-old", field.Before)
	assert.Equal(t, "db-new", field.After)
}

func TestDeploymentDiffNeverShowsCredentialEnvValues(t *testing.T) {
	before, after := editedDeployment(func(c *corev1.Container) {
		c.Env = []corev1.EnvVar{
			{Name: "DB_PASSWORD", Value: "hunter2-new"},
			{Name: "API_TOKEN", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "creds"}, Key: "token"}}},
		}
	})
	before.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "DB_PASSWORD", Value: "hunter2-old"}}

	got := fieldsByPath(kube.DeploymentSchema().Diff(before, after))

	assert.Equal(t, "changed", got["containers[app].env.DB_PASSWORD"].After)
	assert.Equal(t, "changed", got["containers[app].env.DB_PASSWORD"].Before)
	ref := got["containers[app].env.API_TOKEN"]
	assert.Equal(t, "from secret creds/token", ref.After,
		"a reference names the Secret and key, never a value")
	for _, f := range got {
		assert.NotContains(t, f.Before+f.After, "hunter2")
	}
}

func TestDeploymentDiffReportsCommandProbesAndVolumes(t *testing.T) {
	before, after := editedDeployment(func(c *corev1.Container) {
		c.Command = []string{"/app", "--mode=new"}
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/ready", Port: intstr.FromInt(8080)}},
			PeriodSeconds: 5, TimeoutSeconds: 1, FailureThreshold: 3,
		}
	})
	before.Spec.Template.Spec.Containers[0].Command = []string{"/app"}
	after.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "cfg",
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.
			ConfigMapVolumeSource{LocalObjectReference: corev1.
			LocalObjectReference{Name: "app-config-v2"}}}}}

	got := fieldsByPath(kube.DeploymentSchema().Diff(before, after))

	assert.Equal(t, "/app", got["containers[app].command"].Before)
	assert.Equal(t, "/app --mode=new", got["containers[app].command"].After)
	assert.Equal(t, "http /ready :8080 every 5s, timeout 1s, 3 failures",
		got["containers[app].readinessProbe"].After)
	assert.Equal(t, "configmap/app-config-v2", got["volumes[cfg]"].After)
}

func TestDeploymentDiffRanksLikeliestCulpritFirst(t *testing.T) {
	before, after := editedDeployment(func(c *corev1.Container) {
		c.Image = "app:2.0"
		c.Args = []string{"--fast"}
		c.Env = []corev1.EnvVar{{Name: "MODE", Value: "x"}}
		c.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("256Mi")}
	})
	before.Spec.Template.Spec.Containers[0].Resources.Limits =
		corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")}

	fields := inventory.RankFields(kube.DeploymentSchema().Diff(before, after))

	var paths []string
	for _, f := range fields {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{
		"containers[app].image", "containers[app].env.MODE",
		"containers[app].args", "containers[app].resources.limits.memory",
	}, paths)
}

func TestTranslatorBoundsLargeChangesAndRedactsValues(t *testing.T) {
	before, after := editedDeployment(func(c *corev1.Container) {
		c.Env = nil
		for i := 0; i < 30; i++ {
			c.Env = append(c.Env, corev1.EnvVar{
				Name:  "VAR_" + string(rune('A'+i%26)) + string(rune('a'+i/26)),
				Value: "v"})
		}
		c.Command = []string{strings.Repeat("x", 500)}
		c.Args = []string{"--url=https://user:s3cretpass@db.example.com/x"}
	})
	before.ResourceVersion, after.ResourceVersion = "1", "2"
	tr := kube.NewTranslator(kube.DeploymentSchema())

	var change inventory.Change
	for _, o := range tr.Updated(before, after, time.Now()) {
		if o.Kind == inventory.Changed {
			change = o.Change
		}
	}

	require.NotEmpty(t, change.Fields)
	assert.LessOrEqual(t, len(change.Fields), 12)
	assert.Equal(t, "(more fields)", change.Fields[len(change.Fields)-1].Path)
	for _, f := range change.Fields {
		assert.LessOrEqual(t, len(f.After), 96+len("…"))
		assert.NotContains(t, f.After, "s3cretpass")
	}
}

func TestIngressDiffReportsHostsAndBackends(t *testing.T) {
	rule := func(host, svc string) networkingv1.Ingress {
		return networkingv1.Ingress{Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{Path: "/",
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: svc, Port: networkingv1.
										ServiceBackendPort{Number: 80}}}}}}}}}}}
	}
	before, after := rule("a.example.com", "web"), rule("b.example.com", "web")

	got := fieldsByPath(kube.IngressSchema{}.Diff(&before, &after))

	assert.Equal(t, "a.example.com/", got["spec.rules"].Before)
	assert.Equal(t, "b.example.com/", got["spec.rules"].After)
	assert.NotContains(t, got, "spec.backends",
		"the Service is the same; only the host moved")
}

func TestRouteDiffReportsHostnamesAndBackends(t *testing.T) {
	route := func(host, svc string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"spec": map[string]any{
				"hostnames": []any{host},
				"rules": []any{map[string]any{"backendRefs": []any{
					map[string]any{"name": svc, "port": int64(80)}}}},
			}}}
	}
	before, after := route("a.example.com", "web"), route("a.example.com", "web2")
	before.SetGeneration(1)
	after.SetGeneration(2)
	schema := kube.NewUnstructuredSchema("gateway.networking.k8s.io",
		"httproute")

	got := fieldsByPath(schema.Diff(before, after))

	assert.Equal(t, "web:80", got["spec.backends"].Before)
	assert.Equal(t, "web2:80", got["spec.backends"].After)
	assert.NotContains(t, got, "spec.hostnames")
}

func TestHPAAndPDBAndNodeDiffs(t *testing.T) {
	cpu := func(n int32) []autoscalingv2.MetricSpec {
		return []autoscalingv2.MetricSpec{{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU, Target: autoscalingv2.MetricTarget{
					AverageUtilization: &n}}}}
	}
	b, a := hpa("h"), hpa("h")
	b.Spec.Metrics, a.Spec.Metrics = cpu(60), cpu(90)
	got := fieldsByPath(kube.HPASchema{}.Diff(b, a))
	assert.Equal(t, "cpu 60%", got["spec.metrics"].Before)
	assert.Equal(t, "cpu 90%", got["spec.metrics"].After)

	min1, min2 := intstr.FromInt(1), intstr.FromInt(2)
	pb := &policyv1.PodDisruptionBudget{Spec: policyv1.
		PodDisruptionBudgetSpec{MinAvailable: &min1}}
	pa := &policyv1.PodDisruptionBudget{Spec: policyv1.
		PodDisruptionBudgetSpec{MinAvailable: &min2}}
	got = fieldsByPath(kube.PDBSchema{}.Diff(pb, pa))
	assert.Equal(t, "1", got["spec.minAvailable"].Before)
	assert.Equal(t, "2", got["spec.minAvailable"].After)

	nb, na := node("n1"), node("n1")
	nb.Labels = map[string]string{"pool": "a", "gone": "x"}
	na.Labels = map[string]string{"pool": "b"}
	got = fieldsByPath(kube.NodeSchema{}.Diff(nb, na))
	assert.Equal(t, "b", got["labels.pool"].After)
	assert.Equal(t, "x", got["labels.gone"].Before)
}

func TestNetworkPolicyDiffSummarisesRules(t *testing.T) {
	b := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	a := b.DeepCopy()
	a.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
	a.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}

	got := fieldsByPath(kube.NetworkPolicySchema{}.Diff(b, a))

	assert.Equal(t, "1 rules", got["spec.egress"].After)
	assert.Equal(t, "Egress", got["spec.policyTypes"].After)
}
