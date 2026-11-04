package explain

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const attacherFinalizer = "external-attacher/csi-example-com"

// stuckAttachments adds n volume attachments held by the driver's
// attacher finalizer, each with the given detach finding evidence.
func (f *fixture) stuckAttachments(
	n int, detach ...detection.Evidence,
) []inventory.EntityID {
	var out []inventory.EntityID
	for i := 0; i < n; i++ {
		id := inventory.CoreID(kube.KindVolumeAttachment, "",
			"csi-"+strconv.Itoa(i))
		f.add(id)
		f.findings[id] = append(f.findings[id], detection.Finding{
			Entity: id, Reason: "StuckDeleting",
			Mode: detection.ModeStuckDeleting, Health: degradedH,
			Since: t0.Add(-24 * time.Hour), Summary: "Deletion is blocked",
			Evidence: []detection.Evidence{{Label: "finalizers",
				Value: attacherFinalizer}},
		}, detection.Finding{
			Entity: id, Reason: "VolumeDetachFailure",
			Mode: detection.ModeVolumeDetachFailed, Health: failingH,
			Since: t0.Add(-24 * time.Hour), Summary: "Volume cannot be " +
				"detached from its node", Evidence: detach,
		})
		out = append(out, id)
	}
	return out
}

// The driver's attacher runs but cannot detach a volume whose PV and
// node are gone: the finalizer is not unhandled, and the attachments
// are their own roots.
func TestFinalizerOfAnAttachmentWhoseVolumeIsGoneIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	held := f.stuckAttachments(2, detection.Evidence{
		Label: detection.EvidenceAttachGone, Value: "volume and node"})

	c, ok := rootOf(f, held[0])

	assert.False(t, ok && c.Root.Kind == KindFinalizer,
		"cause %v names the finalizer", c.Root)
}

func TestFinalizerOfAnAttachmentWithADetachErrorIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	held := f.stuckAttachments(2, detection.Evidence{
		Label: detection.EvidenceDetachError,
		Value: "persistentvolume \"pv-1\" not found"})

	c, ok := rootOf(f, held[0])

	assert.False(t, ok && c.Root.Kind == KindFinalizer)
}

// Without a detach error or anything gone, nothing says why the
// attachments stay: the finalizer leftover is still the cause.
func TestFinalizerOfAttachmentsWithoutDetachEvidenceIsBlamed(t *testing.T) {
	f := newFixture(t)
	held := f.stuckAttachments(2)

	c, ok := rootOf(f, held[0])

	assert.True(t, ok && c.Root.Kind == KindFinalizer, "cause %v", c.Root)
}
