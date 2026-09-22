//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodCrashLoop(t *testing.T) {
	runScenario(t, "pod.crashloop", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		_, err := e.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(
				context.Background(), 2*time.Minute,
			)
			defer cancel()
			if err := e.DeleteNamespace(cleanupCtx, namespace); err != nil {
				t.Errorf("delete scenario namespace: %v", err)
			}
		}()
		pod, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "crashloop"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:            "workload",
				Image:           workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "crash"},
				ImagePullPolicy: corev1.PullNever,
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		podCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(podCtx, namespace, pod.Name, func(
			pod *corev1.Pod,
		) bool {
			return harness.PodHasReason(pod, "CrashLoopBackOff")
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.Health.AssertOK(ctx, "/healthz"); err != nil {
			t.Fatal(err)
		}
		if err := e.Health.AssertOK(ctx, "/readyz"); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
		entries, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "crashloop",
			Reason:    "CrashLoopBackOff",
			Count:     1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected one incident, got %d", len(entries))
		}
	})
}
