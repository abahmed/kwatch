package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func misscheduledDaemonSet(
	misscheduled float64,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindDaemonSet, "monitoring", "agent")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDesiredReplicas: inventory.Number(3),
		kube.AttrReadyReplicas:   inventory.Number(3),
		kube.AttrMisscheduled:    inventory.Number(misscheduled),
	})
	return m, id
}

func TestWorkloadDaemonSetMisscheduledIsADigestFindingAfterTheGrace(
	t *testing.T,
) {
	m, id := misscheduledDaemonSet(2)
	r := detection.NewRegistry(nil, NewWorkload(0))

	early := r.Evaluate(m, t0.Add(time.Minute), id)
	late := r.Evaluate(m, t0.Add(DefaultUnavailable), id)

	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultUnavailable-time.Minute, early.RecheckAfter)
	require.Len(t, late.Findings, 1)
	f := late.Findings[0]
	assert.Equal(t, reasons.DaemonSetMisscheduled, f.Reason)
	assert.Equal(t, detection.Info, f.Severity)
	assert.Contains(t, f.Summary, "2 nodes")
	assert.Contains(t, f.Evidence,
		detection.Evidence{Label: "misscheduled nodes", Value: "2"})
	assert.True(t, detection.DigestOnly(f.Reason))
}

func TestWorkloadDaemonSetWithoutMisscheduledNodesIsQuiet(t *testing.T) {
	m, id := misscheduledDaemonSet(0)
	r := detection.NewRegistry(nil, NewWorkload(0))

	got := r.Evaluate(m, t0.Add(time.Hour), id)

	assert.Empty(t, got.Findings)
}

func TestWorkloadDaemonSetMisscheduledSummaryAgreesWithTheCount(t *testing.T) {
	m, id := misscheduledDaemonSet(1)
	r := detection.NewRegistry(nil, NewWorkload(0))

	got := r.Evaluate(m, t0.Add(DefaultUnavailable), id)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, "1 node runs its pod although it is not meant to "+
		"run there", got.Findings[0].Summary)
}
