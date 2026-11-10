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

// horizon is the old-problem horizon of these tests.
const horizon = 30 * 24 * time.Hour

// oldSig is a finding whose onset was age before t0.
func oldSig(
	name, why string, sev detection.Severity, age time.Duration,
) detection.Finding {
	s := sig(entity(kube.KindJob, name), why, sev)
	s.Since = t0.Add(-age)
	return s
}

// A low finding that was already older than the horizon when kwatch
// first saw it opens no incident; it is only counted.
func TestOldLowFindingOpensNoIncident(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	r.raise(at(0), old)

	assert.Empty(t, r.m.Export())
	got := r.m.OldProblems()
	require.Len(t, got, 1)
	assert.Equal(t, old.Entity, got[0].Entity)

	r.clear(at(time.Minute), old)
	assert.Empty(t, r.m.OldProblems(), "a cleared one is not counted")
}

// Zero turns the age-out off, and a young low finding opens as usual.
func TestYoungOrUnconfiguredLowFindingOpens(t *testing.T) {
	off := newRig(t, Config{})
	off.raise(at(0), oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour))
	assert.Len(t, off.m.Export(), 1)

	on := newRig(t, Config{OldProblem: horizon})
	on.raise(at(0), oldSig("report", reasons.JobFailed, detection.Info,
		horizon-time.Hour))
	assert.Len(t, on.m.Export(), 1)
	assert.Empty(t, on.m.OldProblems())
}

// An old finding that gets worse, or starts again, opens normally.
func TestOldFindingThatChangesOpens(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	r.raise(at(0), old)
	worse := old
	worse.Severity = detection.Warning
	r.apply(at(time.Minute), detection.Changed, worse)

	assert.Len(t, r.m.Export(), 1)
	assert.Empty(t, r.m.OldProblems())

	again := newRig(t, Config{OldProblem: horizon})
	again.raise(at(0), old)
	fresh := old
	fresh.Since = at(time.Minute)
	again.apply(at(time.Minute), detection.Changed, fresh)
	assert.Len(t, again.m.Export(), 1)
}

// A Notify-level problem that had already lasted past the horizon when
// its incident opened is Low, and says why in plain words.
func TestChronicNotifyIsLoweredToLow(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), oldSig("backup", reasons.JobFailed, detection.Warning,
		2*365*24*time.Hour))

	p := r.only()
	assert.Equal(t, Digest, p.Tier)
	assert.Equal(t, "Lowered to Low: already failing for about 2 years "+
		"when kwatch first saw it, and nothing has changed since. "+
		"kwatch lists it in the daily summary and sends no alert; a new "+
		"failure makes it Needs attention again.",
		WhyOf(p, at(time.Minute)).AlertLevel)
}

// A new onset beside the old one keeps the incident Needs attention,
// and a young problem was never lowered.
func TestNewOnsetKeepsNotify(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), oldSig("backup", reasons.JobFailed, detection.Warning,
		40*24*time.Hour))
	require.Equal(t, Digest, r.only().Tier)

	young := sig(entity(kube.KindJob, "backup"), reasons.CrashLoopBackOff,
		detection.Warning)
	young.Since = at(time.Minute)
	r.raise(at(time.Minute), young)
	assert.Equal(t, Notify, r.only().Tier)

	fresh := newRig(t, Config{OldProblem: horizon})
	fresh.raise(at(0), oldSig("backup", reasons.JobFailed,
		detection.Warning, time.Hour))
	assert.Equal(t, Notify, fresh.only().Tier)
}

// A critical old failure is never lowered.
func TestChronicCriticalKeepsItsTier(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), oldSig("backup", reasons.JobFailed, detection.Critical,
		400*24*time.Hour))
	assert.GreaterOrEqual(t, r.only().Tier, Notify)
}

// An announced Notify incident that turns out chronic (restored from a
// release without the rule) falls to the digest without a message.
func TestAnnouncedChronicFallsQuietly(t *testing.T) {
	src := newRig(t, Config{})
	old := oldSig("backup", reasons.JobFailed, detection.Warning,
		400*24*time.Hour)
	announced(t, src, old)
	require.Equal(t, Notify, src.only().Tier)

	dst := newRig(t, Config{OldProblem: horizon})
	grace := at(10 * time.Minute)
	dst.m.Restore(src.m.Export(), grace)
	dst.raise(at(time.Minute), old)
	wantNone(t, dst.tick(grace))
	wantNone(t, dst.tick(grace.Add(DefaultReviseSettle)))
	assert.Equal(t, Digest, dst.only().Tier)
}

func TestSpokenDays(t *testing.T) {
	for days, want := range map[int]string{
		45: "45 days", 75: "about 2 months", 400: "about a year",
		900: "about 2 years",
	} {
		assert.Equal(t, want, spokenDays(days))
	}
}

// Old leftovers that one cause explains together stay one Low incident:
// a group is not hidden member by member.
func TestOldGroupedFindingsStillOpenOneIncident(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	controller := entity(kube.KindDeployment, "controller")
	var stuck []detection.Finding
	for _, name := range []string{"a", "b"} {
		s := oldSig(name, reasons.StuckDeleting, detection.Info,
			400*24*time.Hour)
		r.cause(s.Entity, controller, "scaled to zero")
		stuck = append(stuck, s)
	}
	r.raise(at(0), stuck...)

	assert.Len(t, r.of(controller).Members, 2)
	assert.Empty(t, r.m.OldProblems())
}

// worsened is an old Info finding that kwatch left out as an old
// problem and that then got worse: it opens at Needs attention.
func worsened(t *testing.T) (*rig, detection.Finding) {
	t.Helper()
	r := newRig(t, Config{OldProblem: horizon})
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	r.raise(at(0), old)
	require.Len(t, r.m.OldProblems(), 1)
	worse := old
	worse.Severity = detection.Warning
	r.apply(at(time.Minute), detection.Changed, worse)
	return r, worse
}

// An old low finding that gets worse is news, though its onset is
// still the old one: it is not lowered for its age.
func TestOldLowFindingThatGetsWorseIsNotLowered(t *testing.T) {
	r, _ := worsened(t)

	p := r.only()
	assert.Equal(t, Notify, p.Tier)
	assert.NotContains(t, p.Journal.Level, levelChronicPrefix)
}

// The mark is saved: after a restart the same finding, with the same
// old onset, is still not lowered.
func TestWorsenedOldFindingStaysUpAfterARestart(t *testing.T) {
	r, worse := worsened(t)
	recs := r.m.Export()
	require.True(t, recs[0].Rose)

	fresh := newRig(t, Config{OldProblem: horizon})
	fresh.m.Restore(recs, at(2*time.Minute))
	fresh.raise(at(2*time.Minute), worse)

	assert.Equal(t, Notify, fresh.only().Tier)
}

// A low member of an incident lowered for its age that gets worse
// raises the incident, too.
func TestMemberThatGetsWorseEndsTheLowering(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), oldSig("backup", reasons.JobFailed, detection.Warning,
		400*24*time.Hour))
	low := oldSig("backup", reasons.JobBackoffLimitExceeded, detection.Info,
		400*24*time.Hour)
	r.raise(at(time.Minute), low)
	require.Equal(t, Digest, r.only().Tier)
	require.Len(t, r.only().Members, 2)

	low.Severity = detection.Warning
	r.apply(at(2*time.Minute), detection.Changed, low)

	assert.Equal(t, Notify, r.only().Tier)
}
