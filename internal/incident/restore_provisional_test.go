package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// restoredMissingTarget is an autoscaler incident told long ago and since
// carried by the digest as HPATargetMissing, restored after a restart at
// restartAt with a ten-minute grace and no findings back yet.
func restoredMissingTarget(t *testing.T) (*rig, time.Time) {
	return restoredMissingTargetWith(t, Config{})
}

func restoredMissingTargetWith(
	t *testing.T, cfg Config,
) (*rig, time.Time) {
	t.Helper()
	old := newRig(t, Config{})
	announced(t, old, scaleFailure())
	now := at(2 * time.Minute)
	old.raise(now, missingTarget())
	old.clear(now, scaleFailure())
	old.tick(now)
	wantAction(t, old.tick(now.Add(DefaultReviseSettle)),
		Update, ReasonCauseRevised)
	require.Equal(t, Digest, old.only().Tier)
	recs := old.m.Export()
	require.Equal(t, []string{reasons.HPATargetMissing}, recs[0].Reasons)

	restartAt := at(time.Hour)
	fresh := newRig(t, cfg)
	fresh.model = old.model
	fresh.m.Restore(recs, restartAt.Add(10*time.Minute))
	return fresh, restartAt
}

// oomCritical makes an OOMKilled finding page.
var oomCritical = Config{SeverityByReason: map[string]string{
	reasons.OOMKilled: "critical"}}

// Right after a restart the Deployment kind is not synced, so the
// autoscaler shows only the event finding (notify). The saved reason is
// not back: the rise is provisional and nothing is said. When the saved
// reason returns the tier follows it back and nothing is said either.
func TestRestoreProvisionalRiseIsNotToldWhileReasonsReturn(
	t *testing.T,
) {
	r, restartAt := restoredMissingTarget(t)
	r.raise(restartAt.Add(2*time.Second), scaleFailure())
	assert.Equal(t, Notify, r.only().Tier, "the tier follows the members")
	wantNone(t, r.tick(restartAt.Add(3*time.Second)))
	wantNone(t, r.tick(restartAt.Add(time.Minute)))

	back := restartAt.Add(2 * time.Minute)
	// One evaluation: the detector words the failure as before.
	delete(r.active, scaleFailure().Key())
	r.active[missingTarget().Key()] = missingTarget()
	r.m.Apply(r.snapshot(back), nil, []detection.Transition{
		{Kind: detection.Raised, Finding: missingTarget()},
		{Kind: detection.Cleared, Finding: scaleFailure()},
	})
	assert.Equal(t, Digest, r.only().Tier, "back to what was told")
	for d := time.Duration(0); d <= time.Hour; d += time.Minute {
		wantNone(t, r.tick(back.Add(d)))
	}
	assert.Equal(t, Digest, r.only().Tier)
}

// A rise that still holds when the grace ends is a real one: it is told
// then, once.
func TestRestoreProvisionalRiseIsToldWhenItOutlastsTheGrace(
	t *testing.T,
) {
	r, restartAt := restoredMissingTarget(t)
	r.raise(restartAt.Add(2*time.Second), scaleFailure())

	var inGrace []Decision
	for d := time.Second; d < 10*time.Minute; d += 30 * time.Second {
		inGrace = append(inGrace, r.tick(restartAt.Add(d))...)
	}
	assert.Empty(t, inGrace, "nothing inside the grace")
	_, ok := r.m.deadline(r.m.incidents[r.only().ID], restartAt)
	assert.True(t, ok, "the end of the grace is a deadline")

	ds := r.tick(restartAt.Add(10 * time.Minute))
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
	wantNone(t, r.tick(restartAt.Add(time.Hour)))
}

// Once the saved reasons are back the view is complete, and a rise on
// top of them is told at once, a page included.
func TestRestoreProvisionalEndsWhenTheSavedReasonsAreBack(t *testing.T) {
	r, restartAt := restoredMissingTargetWith(t, oomCritical)
	now := restartAt.Add(time.Minute)
	critical := scaleFailure()
	critical.Reason = reasons.OOMKilled
	critical.Severity = detection.Critical
	r.raise(now, missingTarget(), critical)

	ds := r.tick(now)
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Page, ds[0].Incident.Tier)
}

// A page that rests only on findings other than the saved ones is held
// like any rise: before the saved reasons are back the view is partial.
// It is told when the grace ends if it still holds.
func TestRestoreProvisionalHoldsAPageUntilTheGraceEnds(t *testing.T) {
	r, restartAt := restoredMissingTargetWith(t, oomCritical)
	critical := scaleFailure()
	critical.Reason = reasons.OOMKilled
	critical.Severity = detection.Critical
	r.raise(restartAt.Add(time.Second), critical)
	require.Equal(t, Page, r.only().Tier)

	wantNone(t, r.tick(restartAt.Add(5*time.Minute)))
	ds := r.tick(restartAt.Add(10 * time.Minute))
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Page, ds[0].Incident.Tier)
}

// An older record without saved reasons waits for its root reasons.
func TestRestoreProvisionalFallsBackToRootReasons(t *testing.T) {
	recs := []Record{{State: Open, RootReasons: []string{"A"}}}
	assert.Equal(t, []string{"A"}, awaitedReasons(recs[0]))
	recs[0].Reasons = []string{"B"}
	assert.Equal(t, []string{"B"}, awaitedReasons(recs[0]))
	recs[0].State = Resolved
	assert.Empty(t, awaitedReasons(recs[0]))
}
