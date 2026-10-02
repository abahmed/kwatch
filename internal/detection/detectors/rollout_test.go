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

func stsRollout(updated, partition float64) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(3),
		kube.AttrReadyReplicas:   inventory.Number(3),
		kube.AttrRevision:        inventory.Text("web-2"),
		kube.AttrCurrentRevision: inventory.Text("web-1"),
		kube.AttrCurrentReplicas: inventory.Number(3 - updated),
		kube.AttrUpdatedReplicas: inventory.Number(updated),
		kube.AttrPartition:       inventory.Number(partition),
		kube.AttrUpdateStrategy:  inventory.Text("RollingUpdate"),
	}
}

func rolloutFindings(
	eval detection.Evaluation,
) []detection.Finding {
	var out []detection.Finding
	for _, f := range eval.Findings {
		if f.Reason == reasons.StatefulSetRolloutStuck {
			out = append(out, f)
		}
	}
	return out
}

func TestWorkloadStatefulSetRolloutStuckNamesUnreadyOrdinal(t *testing.T) {
	m := newTestModel()
	sts := newID(kube.KindStatefulSet, "ns", "web")
	put(m, sts, t0, stsRollout(1, 0))
	pod := newID(kube.KindPod, "ns", "web-2")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false),
	})
	link(m, pod, inventory.OwnedBy, sts)

	eval := evaluate(NewWorkload(0), m, t0.Add(11*time.Minute), sts, nil)

	found := rolloutFindings(eval)
	require.Len(t, found, 1)
	assert.Equal(t, "Rollout.Stuck", string(found[0].Mode))
	assert.Equal(t, detection.Failing, found[0].Health)
	assert.Contains(t, found[0].Summary, "web-2 not ready")
	assert.Contains(t, found[0].Evidence, detection.Evidence{
		Label: "current revision", Value: "web-1"})
	assert.Contains(t, found[0].Evidence, detection.Evidence{
		Label: "current replicas", Value: "2"})
}

func TestWorkloadStatefulSetRolloutWaitsForGrace(t *testing.T) {
	m := newTestModel()
	sts := newID(kube.KindStatefulSet, "ns", "web")
	put(m, sts, t0, stsRollout(1, 0))

	eval := evaluate(NewWorkload(0), m, t0.Add(time.Minute), sts, nil)

	assert.Empty(t, rolloutFindings(eval))
	assert.Equal(t, 9*time.Minute, eval.RecheckAfter)
}

func TestWorkloadStatefulSetRolloutHeldByPartition(t *testing.T) {
	m := newTestModel()
	sts := newID(kube.KindStatefulSet, "ns", "web")
	put(m, sts, t0, stsRollout(1, 2))

	eval := evaluate(NewWorkload(0), m, t0.Add(time.Minute), sts, nil)

	found := rolloutFindings(eval)
	require.Len(t, found, 1)
	assert.Equal(t, detection.Info, found[0].Severity)
	assert.Contains(t, found[0].Summary, "partition 2")
}

func TestWorkloadStatefulSetRolloutIgnoresSettledAndOnDelete(t *testing.T) {
	m := newTestModel()
	settled := newID(kube.KindStatefulSet, "ns", "settled")
	attrs := stsRollout(3, 0)
	attrs[kube.AttrCurrentRevision] = inventory.Text("web-2")
	put(m, settled, t0, attrs)
	onDelete := newID(kube.KindStatefulSet, "ns", "manual")
	attrs = stsRollout(0, 0)
	attrs[kube.AttrUpdateStrategy] = inventory.Text("OnDelete")
	put(m, onDelete, t0, attrs)
	later := t0.Add(time.Hour)

	for _, id := range []inventory.EntityID{settled, onDelete} {
		eval := evaluate(NewWorkload(0), m, later, id, nil)
		assert.Empty(t, rolloutFindings(eval), id.Name)
	}
}
