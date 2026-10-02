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
