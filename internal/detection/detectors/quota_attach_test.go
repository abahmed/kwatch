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

func TestQuotaReportsNearLimit(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "ns", "compute")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted:      inventory.Text(""),
		kube.AttrQuotaNearLimit: inventory.Text("cpu=85%,pods=90%"),
	})

	eval := evaluate(Quota{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, "Quota.NearLimit", string(f.Mode))
	assert.Equal(t, detection.Degraded, f.Health)
	assert.Contains(t, f.Summary, "cpu=85%, pods=90%")
}

func TestQuotaExhaustedWinsOverNearLimit(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "ns", "compute")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted:      inventory.Text("pods"),
		kube.AttrQuotaNearLimit: inventory.Text("cpu=85%"),
	})
	eval := evaluate(Quota{}, m, t0, id, nil)
	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.ResourceQuotaExhausted, eval.Findings[0].Reason)

	put(m, id, t0, map[string]inventory.Value{})
	assert.Empty(t, evaluate(Quota{}, m, t0, id, nil).Findings)
}

func TestAttachmentReportsDetachError(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindVolumeAttachment, "", "csi-1")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDetachError: inventory.Text("volume is still in use"),
	})

	eval := evaluate(Attachment{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Volume.DetachFailed", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Evidence, detection.Evidence{
		Label: "detach error", Value: "volume is still in use"})
}

func TestAttachmentReportsStuckDetach(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindVolumeAttachment, "", "csi-2")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
	})

	early := evaluate(Attachment{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 9*time.Minute, early.RecheckAfter)

	eval := evaluate(Attachment{}, m, t0.Add(11*time.Minute), id, nil)
	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.VolumeDetachFailure, eval.Findings[0].Reason)
	assert.Contains(t, eval.Findings[0].Summary, "detaching for")
}

func TestVolumeFailedCarriesReclaimEventMessage(t *testing.T) {
	m := newTestModel()
	pv := newID(kube.KindPV, "", "pv-1")
	put(m, pv, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	})
	warn(m, pv, t0, "VolumeFailedDelete", "disk is attached to a VM")

	eval := evaluate(Volume{}, m, t0, pv, nil)

	require.Len(t, eval.Findings, 1)
	assert.Contains(t, eval.Findings[0].Evidence, detection.Evidence{
		Label: "VolumeFailedDelete", Value: "disk is attached to a VM"})
}

// stuckAttachment is a VolumeAttachment deleting since long ago, linked
// to a node and a PV; the caller decides which of them exist.
func stuckAttachment(
	m *inventory.Model, node, pv inventory.EntityID,
) inventory.EntityID {
	id := newID(kube.KindVolumeAttachment, "", "csi-old")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0.Add(-400 * 24 * time.Hour)),
		kube.AttrDetachError:   inventory.Text("persistentvolume not found"),
	})
	link(m, id, inventory.RunsOn, node)
	link(m, id, inventory.References, pv)
	return id
}

func TestAttachmentNamesAnOrphanStuckDeleting(t *testing.T) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "gone-node")
	pv := newID(kube.KindPV, "", "gone-pv")
	id := stuckAttachment(m, node, pv)

	eval := evaluate(Attachment{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, reasons.VolumeDetachFailure, f.Reason)
	assert.Equal(t, detection.Info, f.Severity)
	assert.Equal(t, "VolumeAttachment csi-old is stuck deleting; "+
		"its node and PV no longer exist", f.Summary)
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "detach error", Value: "persistentvolume not found"})
}

// A node that is gone while its volume still exists keeps the volume
// from attaching elsewhere, so it stays a warning.
func TestAttachmentWithLivePVStaysAWarning(t *testing.T) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "gone-node")
	pv := newID(kube.KindPV, "", "live-pv")
	put(m, pv, t0, nil)
	id := stuckAttachment(m, node, pv)

	eval := evaluate(Attachment{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, detection.Warning, eval.Findings[0].Severity)
	assert.Contains(t, eval.Findings[0].Summary,
		"is stuck deleting; its node no longer exists")
}

// Nothing is concluded about a node or PV that may not be listed yet,
// and the attachment is judged again once they are.
func TestAttachmentWaitsForNodesAndPVsToSync(t *testing.T) {
	m := newTestModel()
	id := stuckAttachment(m, newID(kube.KindNode, "", "n"),
		newID(kube.KindPV, "", "p"))
	notSynced := func(inventory.Kind) bool { return false }

	eval := evaluate(Attachment{}, m, t0, id, notSynced)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Volume cannot be detached from its node",
		eval.Findings[0].Summary)
	assert.ElementsMatch(t, []inventory.Kind{kube.KindNode, kube.KindPV},
		eval.Unsynced)
}
