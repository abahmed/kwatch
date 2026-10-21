package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestPodPendingNeverSinceBeforeCreation: a Pending pod cannot have been
// pending before it existed. A start time older than the pod's creation
// (a stale value) must not move the finding before the creation.
func TestPodPendingNeverSinceBeforeCreation(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	created := now.Add(-7 * time.Minute)
	stale := now.Add(-3 * time.Hour)
	scheduled := kube.ConditionKey("PodScheduled")
	pod := buildPod("test", "default", created, map[string]inventory.Value{
		kube.AttrPhase:                      inventory.Text("Pending"),
		kube.AttrCreated:                    inventory.Time(created),
		kube.AttrStartTime:                  inventory.Time(stale),
		scheduled:                           inventory.Text("True"),
		scheduled + kube.AttrConditionSince: inventory.Time(created),
	})
	ctx := testDetectorContext(newTestModel(), now)

	findings := NewPod(PodThresholds{}).Detect(ctx, pod)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PodPending, findings[0].Reason)
	assert.True(t, created.Equal(findings[0].Since), findings[0].Since)
}
