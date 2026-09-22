//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodOOMKilled(t *testing.T) {
	runScenario(t, "pod.oom-killed", func(
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
			ObjectMeta: metav1.ObjectMeta{Name: "oom"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:    "workload",
				Image:   workloadImage(),
				Command: []string{"/kwatch-e2e-workload", "memory"},
				Env: []corev1.EnvVar{{
					Name: "MEMORY_MB", Value: "64",
				}},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
				},
				ImagePullPolicy: corev1.PullNever,
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, namespace, "oom", func(
			pod *corev1.Pod,
		) bool {
			return harness.PodHasReason(pod, "OOMKilled")
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "oom",
			Reason:    "OOMKilled",
			Count:     1,
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioPodUnschedulable(t *testing.T) {
	runScenario(t, "pod.pending-unschedulable", func(
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
			ObjectMeta: metav1.ObjectMeta{Name: "unschedulable"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:    "workload",
				Image:   workloadImage(),
				Command: []string{"/kwatch-e2e-workload", "sleep"},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1000"),
					},
				},
				ImagePullPolicy: corev1.PullNever,
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, namespace, "unschedulable",
			func(pod *corev1.Pod) bool {
				return pod.Status.Phase == corev1.PodPending &&
					hasPodReason(pod, "Unschedulable")
			},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "unschedulable",
			Reason:    "Unschedulable",
			Count:     1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func hasPodReason(pod *corev1.Pod, expected string) bool {
	for _, condition := range pod.Status.Conditions {
		if string(condition.Reason) == expected {
			return true
		}
	}
	return false
}
