package kube

import (
	"fmt"
	"slices"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
)

var trimTime = metav1.NewTime(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))

func trimBasePod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "testns", UID: types.UID(name),
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "app", Image: "app:1.0"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func trimEnvs(n int) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, corev1.EnvVar{
			Name: fmt.Sprintf("SETTING_%d", i), Value: "plain-value",
		})
	}
	return out
}

// trimRichPod has every field the trim removes or keeps.
func trimRichPod(name string) *corev1.Pod {
	p := trimBasePod(name)
	yes := true
	secretEnv := corev1.EnvVar{Name: "DB_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "db"},
				Key:                  "password", Optional: &yes}}}
	cmEnv := corev1.EnvVar{Name: "MODE", ValueFrom: &corev1.EnvVarSource{
		ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
			Key:                  "mode"}}}
	fieldEnv := corev1.EnvVar{Name: "POD_IP", ValueFrom: &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}}
	env := append(trimEnvs(45), secretEnv, cmEnv, fieldEnv,
		corev1.EnvVar{Name: "DATABASE_URL",
			Value: "postgres://u:p@db.example.com:5432/orders"},
		corev1.EnvVar{Name: "REDIS_HOST", Value: "redis:6379"},
		corev1.EnvVar{Name: "CSI_DRIVER_NAME", Value: "Disk.CSI.Example.com"},
		corev1.EnvVar{Name: "API", Value: "http://api.testns.svc:8080"})
	p.Spec.InitContainers = []corev1.Container{{
		Name: "wait", Image: "busybox",
		Command: []string{"sh", "-c", "until nc -z redis 6379; do sleep 1; done"},
		Env:     []corev1.EnvVar{{Name: "DB_HOST", Value: "db.testns.svc"}},
	}}
	p.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "app:1.0", Env: env,
		Command: []string{"/app", "--driver-name=A.example.com"},
		Args: []string{"--token=secret", "--provisioner", "b.example.com",
			"--verbose", "--other", "x"},
		ImagePullPolicy: corev1.PullAlways, WorkingDir: "/srv",
		TerminationMessagePath: "/dev/termination-log",
		Lifecycle: &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{Command: []string{"sleep", "10"}}}},
		EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "creds"}}}},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("256Mi")}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "cfg", MountPath: "/etc/cfg"},
			{Name: "cfg", MountPath: "/etc/one", SubPath: "one"},
			{Name: "data", MountPath: "/data"}},
		Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
	}, {
		Name: "sidecar", Image: "side:1",
		Args: []string{"--driver", "c.example.com", "--x=y"},
	}}
	p.Spec.Volumes = []corev1.Volume{
		{Name: "cfg", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
				Items:                []corev1.KeyToPath{{Key: "a", Path: "a"}}}}},
		{Name: "tls", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: "tls"}}},
		{Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "data-0"}}},
		{Name: "scratch", VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "host", VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: "/var/log"}}},
		{Name: "kube-api-access", VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
						Path: "token"}},
					{ConfigMap: &corev1.ConfigMapProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "kube-root-ca.crt"}}},
					{Secret: &corev1.SecretProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "extra"}, Optional: &yes}}}}}},
	}
	p.Spec.NodeName = "node-1"
	return p
}

// trimPodSet is the pods the golden test compares.
func trimPodSet() []*corev1.Pod {
	plain := trimBasePod("plain")
	empty := trimBasePod("empty")
	empty.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "A"}}
	empty.Spec.Volumes = []corev1.Volume{{Name: "x"}}
	waiting := trimRichPod("waiting")
	waiting.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name: "wait", State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: "PodInitializing"}}}}
	crashing := trimRichPod("crashing")
	crashing.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app", RestartCount: 9, State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: "CrashLoopBackOff"}}}}
	set := []*corev1.Pod{plain, empty, trimRichPod("rich"), waiting, crashing}
	for _, p := range set {
		p.ManagedFields = []metav1.ManagedFieldsEntry{{
			Manager: "kubectl", Time: &trimTime,
			FieldsV1: &metav1.FieldsV1{Raw: []byte(`{"f:spec":{}}`)}}}
	}
	return set
}

// TestTrimmedPodGivesTheSameObservations is the guard on trimPod: the
// informer cache holds the trimmed pod, and nothing a reader derives
// may change.
func TestTrimmedPodGivesTheSameObservations(t *testing.T) {
	transform := newTransform(NewDigester([]byte("k")))
	translator := NewTranslator(PodSchema{})
	for _, original := range trimPodSet() {
		trimmedIn, err := transform(original.DeepCopy())
		if err != nil {
			t.Fatal(err)
		}
		want := translator.Added(original.DeepCopy(), true, trimTime.Time)
		got := translator.Added(trimmedIn, true, trimTime.Time)
		if len(want) == 0 {
			t.Fatalf("%s: no observations", original.Name)
		}
		if !sameObservations(want, got) {
			t.Errorf("%s: trimmed pod differs", original.Name)
		}
		after := original.DeepCopy()
		after.ResourceVersion = "2"
		trimmedAfter, _ := transform(after.DeepCopy())
		wantUp := translator.Updated(original.DeepCopy(), after, trimTime.Time)
		gotUp := translator.Updated(trimmedIn, trimmedAfter, trimTime.Time)
		if !sameObservations(wantUp, gotUp) {
			t.Errorf("%s: trimmed update differs", original.Name)
		}
	}
}

func TestTrimPodDropsWhatNothingReads(t *testing.T) {
	p := trimRichPod("rich")
	trimPod(p)
	app := p.Spec.Containers[0]
	if len(app.Args) != 0 || app.Lifecycle != nil || app.WorkingDir != "" {
		t.Errorf("container kept unread fields: %+v", app)
	}
	for _, e := range app.Env {
		if e.Name == "SETTING_1" || e.Name == "POD_IP" {
			t.Errorf("env %s should be dropped", e.Name)
		}
	}
	if len(app.VolumeMounts) != 1 {
		t.Errorf("mounts = %+v, want only the sub-path mount", app.VolumeMounts)
	}
	for _, v := range p.Spec.Volumes {
		if v.EmptyDir != nil || v.HostPath != nil {
			t.Errorf("volume %s kept its source", v.Name)
		}
	}
	if len(p.Spec.InitContainers[0].Command) != 3 {
		t.Error("init container words must stay: init_wait reads them")
	}
}

// sameObservations compares two observation lists as sets: the order
// the translator emits related observations in is not significant.
func sameObservations(a, b []inventory.Observation) bool {
	text := func(list []inventory.Observation) []string {
		out := make([]string, 0, len(list))
		for _, o := range list {
			out = append(out, fmt.Sprintf("%+v", o))
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(text(a), text(b))
}
