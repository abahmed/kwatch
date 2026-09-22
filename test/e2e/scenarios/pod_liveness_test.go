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

func TestScenarioPodLivenessFailure(t *testing.T) {
	runScenario(t, "pod.liveness-failure", func(
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
			ObjectMeta: metav1.ObjectMeta{Name: "liveness"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "http"},
				ImagePullPolicy: corev1.PullNever,
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
						Path: "/missing", Port: intstr.FromInt(8080),
					}},
					InitialDelaySeconds: 1,
					PeriodSeconds:       1,
					FailureThreshold:    2,
				},
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, namespace, "liveness",
			func(pod *corev1.Pod) bool {
				return hasContainerOrPodReason(pod, "LivenessProbeFailed") ||
					pod.Status.ContainerStatuses != nil &&
						pod.Status.ContainerStatuses[0].RestartCount > 0
			},
		); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
	})
}
