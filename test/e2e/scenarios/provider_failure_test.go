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

func TestScenarioProviderFailureRecovery(t *testing.T) {
	runScenario(t, "lifecycle.provider-failure", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		if err := e.Receiver.SetPolicy(ctx, map[string]string{
			"mode": "http-500",
		}); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = e.Receiver.SetPolicy(context.Background(), map[string]string{
				"mode": "success",
			})
		}()
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "provider-failure"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "crash"},
				ImagePullPolicy: corev1.PullNever,
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, namespace, "provider-failure",
			func(pod *corev1.Pod) bool {
				return harness.PodHasReason(pod, "CrashLoopBackOff")
			},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "provider-failure",
			Reason: "CrashLoopBackOff", Action: "create", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		requests, err := e.Receiver.WaitForCount(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if requests[0].Status < 500 {
			t.Fatalf("expected provider failure, got HTTP %d", requests[0].Status)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
		if err := e.Receiver.SetPolicy(ctx, map[string]string{
			"mode": "success",
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}
