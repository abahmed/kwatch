//go:build e2e

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioNodePressureSignals(t *testing.T) {
	runScenario(t, "node.pressure-signals", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		if os.Getenv("KIND_CLUSTER_NAME") == "" {
			t.Skip("KIND_CLUSTER_NAME is required for node scenarios")
		}
		node, err := firstWorkerNode(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		if err := stopKindNode(ctx, node.Name); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = startKindNode(context.Background(), node.Name) }()
		conditions := []corev1.NodeConditionType{
			corev1.NodeMemoryPressure,
			corev1.NodeDiskPressure,
			corev1.NodePIDPressure,
			corev1.NodeNetworkUnavailable,
		}
		if err := patchPressureConditions(ctx, e, node.Name, conditions); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 7*time.Minute)
		defer cancel()
		for _, condition := range conditions {
			if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
				Resource: node.Name, Reason: string(condition), Count: 1,
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := startKindNode(ctx, node.Name); err != nil {
			t.Fatal(err)
		}
		if err := waitForNodeReady(waitCtx, e, node.Name, true); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func patchPressureConditions(
	ctx context.Context,
	e *harness.Environment,
	name string,
	conditions []corev1.NodeConditionType,
) error {
	now := metav1.Now()
	items := make([]map[string]any, 0, len(conditions))
	for _, condition := range conditions {
		items = append(items, map[string]any{
			"type": string(condition), "status": string(corev1.ConditionTrue),
			"reason": "KwatchE2E", "message": "controlled scenario pressure",
			"lastHeartbeatTime": now, "lastTransitionTime": now,
		})
	}
	patch, err := json.Marshal(map[string]any{
		"status": map[string]any{"conditions": items},
	})
	if err != nil {
		return fmt.Errorf("marshal node pressure patch: %w", err)
	}
	_, err = e.Client.CoreV1().Nodes().Patch(
		ctx, name, types.StrategicMergePatchType, patch,
		metav1.PatchOptions{}, "status",
	)
	return err
}
