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

// rollingDaemonSet has ten nodes, nine pods ready and updated of them,
// as in a rolling update with maxUnavailable 1. The updated count last
// moved at movedAt.
func rollingDaemonSet(
	updated float64, movedAt time.Time,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindDaemonSet, "kube-system", "agent")
	attrs := map[string]inventory.Value{
		kube.AttrDesiredReplicas: inventory.Number(10),
		kube.AttrReadyReplicas:   inventory.Number(9),
		kube.AttrGeneration:      inventory.Number(2),
		kube.AttrObservedGen:     inventory.Number(2),
		kube.AttrUpdatedReplicas: inventory.Number(updated),
	}
	put(m, id, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrDesiredReplicas: inventory.Number(10),
		kube.AttrReadyReplicas:   inventory.Number(9),
		kube.AttrGeneration:      inventory.Number(2),
		kube.AttrObservedGen:     inventory.Number(2),
		kube.AttrUpdatedReplicas: inventory.Number(0),
	})
	put(m, id, movedAt, attrs)
	return m, id
}

func TestDaemonSetRollingUpdateIsNotReportedWhileItMoves(t *testing.T) {
	m, id := rollingDaemonSet(6, t0.Add(-time.Minute))

	got := evaluate(NewWorkload(5*time.Minute), m, t0, id, nil)

	assert.Empty(t, got.Findings)
	assert.Equal(t, DefaultRolloutStuck-time.Minute, got.RecheckAfter)
}

func TestDaemonSetStuckRolloutIsReported(t *testing.T) {
	m, id := rollingDaemonSet(6, t0.Add(-DefaultRolloutStuck))

	got := evaluate(NewWorkload(5*time.Minute), m, t0, id, nil)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, reasons.DaemonSetUnavailable, got.Findings[0].Reason)
}

func TestDaemonSetFullyUpdatedButUnavailableIsReported(t *testing.T) {
	m, id := rollingDaemonSet(10, t0.Add(-time.Minute))
	put(m, id, t0.Add(-DefaultRolloutStuck), map[string]inventory.Value{
		kube.AttrDesiredReplicas: inventory.Number(10),
		kube.AttrReadyReplicas:   inventory.Number(9),
		kube.AttrGeneration:      inventory.Number(2),
		kube.AttrObservedGen:     inventory.Number(2),
		kube.AttrUpdatedReplicas: inventory.Number(10),
	})

	got := evaluate(NewWorkload(5*time.Minute), m, t0, id, nil)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, reasons.DaemonSetUnavailable, got.Findings[0].Reason)
}
