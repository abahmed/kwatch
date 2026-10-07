package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func scaleFailure() detection.Finding {
	return sig(entity(kube.KindHPA, "thirdparty"),
		reasons.FailedGetScale, detection.Warning)
}

func missingTarget() detection.Finding {
	return sig(entity(kube.KindHPA, "thirdparty"),
		reasons.HPATargetMissing, detection.Info)
}

// An announced incident whose only finding is replaced by a digest-class
// one falls to the digest tier, and its thread hears the new cause once.
func TestManagerReplacedFindingLowersTierAndSaysSo(t *testing.T) {
	r := newRig(t, Config{})
	old := scaleFailure()
	announced(t, r, old)
	require.Equal(t, Notify, r.only().Tier)
	// Delivery marks every own message as reaching the pagers; that alone
	// is no page.
	r.m.RecordPaged(r.only().ID, true)

	now := at(2 * time.Minute)
	r.raise(now, missingTarget())
	r.clear(now, old)
	wantNone(t, r.tick(now))
	ds := r.tick(now.Add(DefaultReviseSettle))

	wantAction(t, ds, Update, ReasonCauseRevised)
	assert.Equal(t, Digest, ds[0].Incident.Tier)
	assert.True(t, ds[0].Thread, "the Notify thread hears the cause")
	wantNone(t, r.tick(now.Add(time.Hour)))
}

// A page stays a page whatever replaces its findings: no new page, no
// un-page, and no message for it.
func TestManagerPagedIncidentIsNeverLowered(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	critical := sig(node, reasons.NodeNotReady, detection.Critical)
	r.raise(at(0), critical)
	wantAction(t, r.tick(at(DefaultPageSettle)), Announce, "settled")
	require.Equal(t, Page, r.of(node).Tier)

	r.raise(at(time.Minute), sig(node, reasons.ContainerCPUHigh,
		detection.Warning))
	r.clear(at(time.Minute), critical)
	for _, d := range r.tick(at(time.Minute + DefaultReviseSettle)) {
		assert.NotEqual(t, ReasonCauseRevised, d.Reason)
		assert.False(t, d.Thread)
	}
	assert.Equal(t, Page, r.of(node).Tier)
}

// A restart keeps no members: the restored Notify incident is judged
// once its findings are back, and a digest-class finding lowers it.
func TestManagerRestoredIncidentLowersWhenFindingsReturnQuieter(t *testing.T) {
	src := newRig(t, Config{})
	announced(t, src, scaleFailure())

	dst := newRig(t, Config{})
	grace := at(10 * time.Minute)
	dst.m.Restore(src.m.Export(), grace)
	dst.raise(at(time.Minute), missingTarget())
	wantNone(t, dst.tick(at(time.Minute)))
	wantNone(t, dst.tick(grace.Add(-time.Second)))
	assert.Equal(t, Notify, dst.only().Tier, "judged after the grace")

	wantNone(t, dst.tick(grace))
	ds := dst.tick(grace.Add(DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonCauseRevised)
	assert.Equal(t, Digest, ds[0].Incident.Tier)
	assert.True(t, ds[0].Thread)

	// The lowered tier survives another restart.
	again := newRig(t, Config{})
	again.m.Restore(dst.m.Export(), time.Time{})
	restored := again.only()
	assert.True(t, restored.Delivery.Demoted())
}

// Going quieter is no reason to stop hearing about worse: a member that
// fails for real raises the lowered incident exactly as before.
func TestManagerLoweredIncidentStillRises(t *testing.T) {
	r := newRig(t, Config{})
	old := scaleFailure()
	announced(t, r, old)
	now := at(2 * time.Minute)
	r.raise(now, missingTarget())
	r.clear(now, old)
	r.tick(now)
	wantAction(t, r.tick(now.Add(DefaultReviseSettle)), Update,
		ReasonCauseRevised)

	later := now.Add(time.Hour)
	r.raise(later, sig(entity(kube.KindHPA, "thirdparty"),
		"ScalingFailed", detection.Warning))
	ds := r.tick(later)
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
}

// A finding that is still a real failure keeps the tier: only digest
// findings left behind lower it.
func TestManagerMixedFindingsKeepTheTier(t *testing.T) {
	r := newRig(t, Config{})
	old := scaleFailure()
	announced(t, r, old)
	now := at(2 * time.Minute)
	r.raise(now, missingTarget())
	r.tick(now.Add(DefaultReviseSettle))
	assert.Equal(t, Notify, r.only().Tier)
}
