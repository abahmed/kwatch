package pod

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestRuntimeHandlesQueueKeysAndSourceAvailability(t *testing.T) {
	sink := &runtimeSink{}
	runtime := NewRuntimeWithRuntimeConfig(
		nil, config.RuntimeConfig{}, sink, nil, nil, time.Now,
	)
	if err := runtime.ProcessPod(
		context.Background(), "bad/key/extra", false,
	); err == nil {
		t.Fatal("invalid pod key succeeded")
	}
	if err := runtime.ProcessPod(
		context.Background(), "apps/api", false,
	); err != nil {
		t.Fatalf("unavailable source returned error: %v", err)
	}
	if err := runtime.ProcessPodObject(
		context.Background(), nil, false,
	); err != nil {
		t.Fatalf("nil pod returned error: %v", err)
	}
	if err := runtime.ProcessPodObject(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "api"},
	}, true); err != nil {
		t.Fatalf("deleted pod returned error: %v", err)
	}
	if sink.removed != 1 {
		t.Fatalf("removed = %d, want 1", sink.removed)
	}
	runtime.SetBaseline(nil)
	runtime.SetActiveNodeIncidents(nil)
}

func TestRuntimeConfigurationAndRecoveryHelpers(t *testing.T) {
	runtime := NewRuntimeWithRuntimeConfig(
		nil, config.RuntimeConfig{}, &runtimeSink{}, nil, nil, time.Now,
	)
	if err := runtime.ConfigureSources(RuntimeSources{}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigureSources(RuntimeSources{}); err == nil {
		t.Fatal("second ConfigureSources() succeeded")
	}
	if err := runtime.ProcessPodObject(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api"},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigureSources(RuntimeSources{}); err == nil {
		t.Fatal("late ConfigureSources() succeeded")
	}
	if podUIDFromQueueKey("apps/api#uid") != "uid" ||
		podUIDFromQueueKey("apps/api") != "" {
		t.Fatal("pod UID parsing failed")
	}
}

func TestPodHealthAndRecoveryClassification(t *testing.T) {
	running := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", Ready: true,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}},
		InitContainerStatuses: []corev1.ContainerStatus{{
			Name: "init",
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 0,
			}},
		}},
	}}
	if !isHealthy(running) {
		t.Fatal("running pod was not healthy")
	}
	recovered := recoveredContainers(running)
	if !recovered["app"] || !recovered["init"] || !recovered[""] {
		t.Fatalf("recovered containers = %#v", recovered)
	}
	terminating := running.DeepCopy()
	terminating.DeletionTimestamp = ptrPodTime(
		time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC),
	)
	if got := recoveredContainers(terminating); got != nil {
		t.Fatalf("terminating pod recovered containers = %#v", got)
	}
	failed := running.DeepCopy()
	failed.Status.Phase = corev1.PodFailed
	if isHealthy(failed) {
		t.Fatal("failed pod was healthy")
	}
	waiting := running.DeepCopy()
	waiting.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
	}
	if isHealthy(waiting) {
		t.Fatal("failing waiting pod was healthy")
	}
}

func ptrPodTime(value time.Time) *metav1.Time {
	result := metav1.NewTime(value)
	return &result
}
