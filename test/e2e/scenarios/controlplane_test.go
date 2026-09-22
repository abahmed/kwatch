//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioControlPlaneComponentRecovery(t *testing.T) {
	runScenario(t, "integration.control-plane", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		pod, err := findControlPlanePod(ctx, e, "kube-scheduler")
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Client.CoreV1().Pods(pod.Namespace).Delete(
			ctx, pod.Name, metav1.DeleteOptions{},
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Reason: "SchedulerUnavailable", Action: "create", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Reason: "SchedulerUnavailable", Action: "resolved", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func findControlPlanePod(
	ctx context.Context,
	e *harness.Environment,
	component string,
) (*corev1.Pod, error) {
	for _, label := range []string{"component", "k8s-app"} {
		pods, err := e.Client.CoreV1().Pods("").List(ctx,
			metav1.ListOptions{LabelSelector: label + "=" + component},
		)
		if err != nil {
			return nil, err
		}
		if len(pods.Items) > 0 {
			return &pods.Items[0], nil
		}
	}
	return nil, fmt.Errorf("control-plane pod %q not found", component)
}
