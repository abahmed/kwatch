//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodStartupFailureProfiles(t *testing.T) {
	profiles := []struct {
		name   string
		image  string
		reason string
		spec   func(*corev1.PodSpec)
	}{
		{name: "image-pull", image: "example.invalid/kwatch/missing:never",
			reason: "ImagePullBackOff"},
		{name: "init-container", image: workloadImage(),
			reason: "CrashLoopBackOff", spec: addFailingInit},
	}
	runScenario(t, "pod.startup-additional", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		for _, profile := range profiles {
			t.Run(profile.name, func(t *testing.T) {
				namespace := uniqueNamespace(t.Name())
				if err := createNamespace(ctx, e, namespace); err != nil {
					t.Fatal(err)
				}
				defer cleanupNamespace(t, e, namespace)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: profile.name},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "workload", Image: profile.image,
						Command:         []string{"/kwatch-e2e-workload", "sleep"},
						ImagePullPolicy: corev1.PullNever,
					}}},
				}
				if profile.spec != nil {
					profile.spec(&pod.Spec)
				}
				if _, err := e.Client.CoreV1().Pods(namespace).Create(
					ctx, pod, metav1.CreateOptions{},
				); err != nil {
					t.Fatal(err)
				}
				waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				if err := e.WaitForPod(waitCtx, namespace, profile.name,
					func(pod *corev1.Pod) bool {
						return hasContainerOrPodReason(pod, profile.reason)
					},
				); err != nil {
					t.Fatal(err)
				}
				if err := e.AssertNoRuntimePanic(ctx); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
}

func addFailingInit(spec *corev1.PodSpec) {
	spec.InitContainers = []corev1.Container{{
		Name: "init", Image: workloadImage(),
		Command:         []string{"/kwatch-e2e-workload", "startup-error"},
		ImagePullPolicy: corev1.PullNever,
	}}
}

func hasContainerOrPodReason(pod *corev1.Pod, reason string) bool {
	if harness.PodHasReason(pod, reason) {
		return true
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason ==
			reason {
			return true
		}
		if status.State.Terminated != nil &&
			status.State.Terminated.Reason == reason {
			return true
		}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Reason == reason {
			return true
		}
	}
	return false
}

func TestScenarioPodReadinessFailure(t *testing.T) {
	runScenario(t, "pod.not-ready", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "not-ready"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "not-ready"},
				ImagePullPolicy: corev1.PullNever,
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt(8080)},
				}},
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, namespace, "not-ready",
			func(pod *corev1.Pod) bool {
				return hasContainerOrPodReason(pod, "ReadinessProbeFailed") ||
					pod.Status.Phase == corev1.PodRunning && !podReady(pod)
			},
		); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
