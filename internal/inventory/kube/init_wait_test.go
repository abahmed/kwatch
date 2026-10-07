package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func initPod(status corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{
				Name: "wait-for-db", Image: "busybox",
				Command: []string{"sh", "-c",
					"until nc -z db:5432; do sleep 1; done"},
				Env: []corev1.EnvVar{{Name: "CACHE_HOST", Value: "cache"}},
				Args: []string{"--target=http://auth.tools.svc:8080",
					"--retries=5"},
			}},
			Containers: []corev1.Container{{Name: "app", Image: "api"}},
		},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{status}},
	}
}

func containerAttrs(
	t *testing.T, pod *corev1.Pod, name string,
) map[string]inventory.Value {
	t.Helper()
	described, _ := kube.PodSchema{}.Describe(pod)
	for _, c := range described.Children {
		if c.ID == kube.ContainerID(pod.Namespace, pod.Name, name) {
			return c.Attributes
		}
	}
	t.Fatalf("container %s not described", name)
	return nil
}

func TestInitContainerNamesTheServicesItIsConfiguredToWaitFor(t *testing.T) {
	attrs := containerAttrs(t, initPod(corev1.ContainerStatus{
		Name: "wait-for-db"}), "wait-for-db")
	assert.Equal(t, "shop/cache,shop/db:5432,tools/auth:8080",
		attrs[kube.AttrServiceCalls].AsText(),
		"words that are no address, such as sh and --retries=5, are left")
}

func TestInitContainerRecordsHowLongItsRunTook(t *testing.T) {
	start := metav1.NewTime(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	done := metav1.NewTime(start.Add(45 * time.Second))
	attrs := containerAttrs(t, initPod(corev1.ContainerStatus{
		Name: "wait-for-db", State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				StartedAt: start, FinishedAt: done}}}), "wait-for-db")
	seconds, _ := attrs[kube.AttrInitSeconds].AsNumber()
	assert.Equal(t, 45.0, seconds)

	failed := containerAttrs(t, initPod(corev1.ContainerStatus{
		Name: "wait-for-db", State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 1, StartedAt: start, FinishedAt: done}}}),
		"wait-for-db")
	assert.NotContains(t, failed, kube.AttrInitSeconds,
		"a failed run says nothing about how long a good one takes")
}

func TestServiceRefsInReadsAddressesFromText(t *testing.T) {
	refs := kube.ServiceRefsIn(`waiting for "db:5432"... and 10.0.0.4:80`,
		"shop")
	assert.Equal(t, []kube.ServiceRef{{
		Service: inventory.CoreID(kube.KindService, "shop", "db"),
		Port:    5432}}, refs)
	assert.Empty(t, kube.ServiceRefsIn("starting up", "shop"))
}
