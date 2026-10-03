//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// missingWebhookName is the webhook configuration of
	// CreateWebhookWithoutService.
	missingWebhookName = "kwatch-e2e-missing-webhook"
	// metricsAPIName is the APIService behind `kubectl top` and autoscaling.
	metricsAPIName = "v1beta1.metrics.k8s.io"
	// cleanupTimeout bounds the removal of cluster-wide objects.
	cleanupTimeout = time.Minute
)

var (
	apiServices = schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1",
		Resource: "apiservices",
	}
	volumeAttachments = schema.GroupVersionResource{
		Group: "storage.k8s.io", Version: "v1",
		Resource: "volumeattachments",
	}
)

// emptyService selects Pods that do not exist.
func emptyService(name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "does-not-exist"},
			Ports:    []corev1.ServicePort{{Port: 8080}},
		},
	}
}

// claimWithMissingStorageClass can never be bound.
func claimWithMissingStorageClass(
	name string,
) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr("kwatch-e2e-missing"),
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

// ingressToMissingService routes every path to a Service that does not
// exist.
func ingressToMissingService(name, service string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: service,
			Port: networkingv1.ServiceBackendPort{Number: 8080},
		},
	}
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path: "/", PathType: &pathType, Backend: backend,
					}},
				},
			},
		}}},
	}
}

// denyAllEgressPolicy blocks all outgoing traffic of every Pod.
func denyAllEgressPolicy(name string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
		},
	}
}

// CreateService creates the Service in the scenario namespace.
func (s *Scenario) CreateService(service *corev1.Service) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().Services(s.Namespace).Create(
		s.Ctx, service, metav1.CreateOptions{})
	s.Must(err)
}

// ChangeServiceSelector points the Service at Pods labelled app=app. A
// Service that was never backed is empty on purpose; one that emptied after
// its selector changed was broken by the edit.
func (s *Scenario) ChangeServiceSelector(name, app string) {
	s.T.Helper()
	patch := []byte(`{"spec":{"selector":{"app":"` + app + `"}}}`)
	_, err := s.Env.Client.CoreV1().Services(s.Namespace).Patch(s.Ctx,
		name, types.MergePatchType, patch, metav1.PatchOptions{})
	s.Must(err)
}

// CreateVolumeClaim creates the claim in the scenario namespace.
func (s *Scenario) CreateVolumeClaim(claim *corev1.PersistentVolumeClaim) {
	s.T.Helper()
	claims := s.Env.Client.CoreV1().PersistentVolumeClaims(s.Namespace)
	_, err := claims.Create(s.Ctx, claim, metav1.CreateOptions{})
	s.Must(err)
}

// CreateIngress creates the Ingress in the scenario namespace.
func (s *Scenario) CreateIngress(ingress *networkingv1.Ingress) {
	s.T.Helper()
	_, err := s.Env.Client.NetworkingV1().Ingresses(s.Namespace).Create(
		s.Ctx, ingress, metav1.CreateOptions{})
	s.Must(err)
}

// CreateNetworkPolicy creates the policy in the scenario namespace.
func (s *Scenario) CreateNetworkPolicy(policy *networkingv1.NetworkPolicy) {
	s.T.Helper()
	policies := s.Env.Client.NetworkingV1().NetworkPolicies(s.Namespace)
	_, err := policies.Create(s.Ctx, policy, metav1.CreateOptions{})
	s.Must(err)
}

// EnforceRestrictedPodSecurity makes the namespace reject Pods that do not
// meet the "restricted" Pod Security Standard.
func (s *Scenario) EnforceRestrictedPodSecurity() {
	s.T.Helper()
	patch := []byte(`{"metadata":{"labels":{` +
		`"pod-security.kubernetes.io/enforce":"restricted"}}}`)
	_, err := s.Env.Client.CoreV1().Namespaces().Patch(s.Ctx, s.Namespace,
		types.MergePatchType, patch, metav1.PatchOptions{})
	s.Must(err)
}

// CreateWebhookWithoutService registers a validating webhook whose backend
// Service does not exist and removes it when the test ends.
func (s *Scenario) CreateWebhookWithoutService() {
	s.T.Helper()
	hooks := s.Env.Client.AdmissionregistrationV1().
		ValidatingWebhookConfigurations()
	_, err := hooks.Create(s.Ctx, missingWebhook(s.Namespace),
		metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), cleanupTimeout)
		defer cancel()
		_ = hooks.Delete(ctx, missingWebhookName, metav1.DeleteOptions{})
	})
}

func missingWebhook(
	namespace string,
) *admissionregistrationv1.ValidatingWebhookConfiguration {
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: missingWebhookName},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name: "missing.kwatch.e2e",
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name: "missing-webhook", Namespace: namespace,
				}},
			AdmissionReviewVersions: []string{"v1"},
			FailurePolicy:           ptr(admissionregistrationv1.Ignore),
			SideEffects: ptr(
				admissionregistrationv1.SideEffectClassNone),
		}},
	}
}

// BreakMetricsAPI points the metrics APIService at a Service that does not
// exist. Call the returned function to restore it: namespace deletion waits
// on every API group, so it must be restored before the scenario ends. It
// also restores when the test ends, in case the scenario fails.
func (s *Scenario) BreakMetricsAPI() (restore func()) {
	s.T.Helper()
	services := s.Env.Dynamic.Resource(apiServices)
	apiService, err := services.Get(s.Ctx, metricsAPIName,
		metav1.GetOptions{})
	if err != nil {
		s.T.Fatalf("metrics APIService is required: %v", err)
	}
	backend, found, err := unstructured.NestedMap(
		apiService.Object, "spec", "service")
	if err != nil || !found {
		s.T.Fatal("metrics APIService has no backing Service")
	}
	original := apiService.DeepCopy()
	restore = func() { s.restoreAPIService(original) }
	s.T.Cleanup(restore)
	backend["name"] = "kwatch-e2e-missing-metrics"
	backend["namespace"] = s.Namespace
	s.Must(unstructured.SetNestedMap(
		apiService.Object, backend, "spec", "service"))
	_, err = services.Update(s.Ctx, apiService, metav1.UpdateOptions{})
	s.Must(err)
	return restore
}

// restoreAPIService puts back the spec of an APIService. It runs during
// cleanup, so it ignores errors and uses its own context.
func (s *Scenario) restoreAPIService(original *unstructured.Unstructured) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	services := s.Env.Dynamic.Resource(apiServices)
	current, err := services.Get(ctx, original.GetName(), metav1.GetOptions{})
	if err != nil {
		return
	}
	spec, found, err := unstructured.NestedFieldCopy(original.Object, "spec")
	if err != nil || !found {
		return
	}
	if unstructured.SetNestedField(current.Object, spec, "spec") != nil {
		return
	}
	_, _ = services.Update(ctx, current, metav1.UpdateOptions{})
}

// CreateFailedVolumeAttachment creates a VolumeAttachment, reports an attach
// error on its status and returns its name.
func (s *Scenario) CreateFailedVolumeAttachment() string {
	s.T.Helper()
	nodes, err := s.Env.Client.CoreV1().Nodes().List(s.Ctx,
		metav1.ListOptions{})
	if err != nil || len(nodes.Items) == 0 {
		s.T.Fatalf("storage scenario requires a schedulable node: %v", err)
	}
	name := uniqueNamespace(s.T.Name())
	attachments := s.Env.Dynamic.Resource(volumeAttachments)
	_, err = attachments.Create(s.Ctx, &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "storage.k8s.io/v1", "kind": "VolumeAttachment",
			"metadata": map[string]any{"name": name},
			"spec": map[string]any{
				"attacher": "kwatch-e2e.csi",
				"nodeName": nodes.Items[0].Name,
				"source": map[string]any{
					"persistentVolumeName": "kwatch-e2e-pv"},
			},
		}}, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), cleanupTimeout)
		defer cancel()
		_ = attachments.Delete(ctx, name, metav1.DeleteOptions{})
	})
	patch := []byte(fmt.Sprintf(
		`{"status":{"attachError":{"message":"e2e attach failure",`+
			`"time":"%s"}}}`, time.Now().UTC().Format(time.RFC3339)))
	_, err = attachments.Patch(s.Ctx, name, types.MergePatchType, patch,
		metav1.PatchOptions{}, "status")
	s.Must(err)
	return name
}

// StopCoreDNS scales CoreDNS to zero and scales it back to its original
// size when the test ends.
func (s *Scenario) StopCoreDNS() {
	s.T.Helper()
	deployments := s.Env.Client.AppsV1().Deployments("kube-system")
	scale, err := deployments.GetScale(s.Ctx, "coredns", metav1.GetOptions{})
	s.Must(err)
	original := scale.Spec.Replicas
	scale.Spec.Replicas = 0
	_, err = deployments.UpdateScale(
		s.Ctx, "coredns", scale, metav1.UpdateOptions{})
	s.Must(err)
	s.T.Cleanup(func() { s.restoreCoreDNS(original) })
}

func (s *Scenario) restoreCoreDNS(replicas int32) {
	t := s.T
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	deployments := s.Env.Client.AppsV1().Deployments("kube-system")
	scale, err := deployments.GetScale(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		t.Errorf("read CoreDNS scale: %v", err)
		return
	}
	scale.Spec.Replicas = replicas
	if _, err := deployments.UpdateScale(
		ctx, "coredns", scale, metav1.UpdateOptions{},
	); err != nil {
		t.Errorf("restore CoreDNS: %v", err)
		return
	}
	err = wait.PollUntilContextTimeout(ctx, time.Second, 3*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			current, getErr := deployments.Get(
				ctx, "coredns", metav1.GetOptions{})
			return getErr == nil &&
				current.Status.ReadyReplicas >= replicas, nil
		})
	if err != nil {
		t.Errorf("wait for CoreDNS: %v", err)
	}
}
