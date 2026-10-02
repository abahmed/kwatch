//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodCrashLoop(t *testing.T) {
	runScenario(t, "pod.crashloop", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		started := time.Now()
		if err := createFailingDeployment(
			ctx, e, namespace, "crashloop",
		); err != nil {
			t.Fatal(err)
		}
		podCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPodCount(podCtx, namespace, func(
			pods []corev1.Pod,
		) bool {
			return len(pods) == 1 &&
				harness.PodHasReason(&pods[0], "CrashLoopBackOff")
		}); err != nil {
			t.Fatal(err)
		}
		assertRoot(ctx, t, e, namespace, started, harness.RootExpectation{
			Root:         "deployment/" + namespace + "/crashloop",
			Tier:         "notify",
			MaxMessages:  2,
			MustNotBlame: scheduledNodes(ctx, t, e, namespace),
		})
		if err := e.Health.AssertOK(ctx, "/healthz"); err != nil {
			t.Fatal(err)
		}
		if err := e.Health.AssertOK(ctx, "/readyz"); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
	})
}
