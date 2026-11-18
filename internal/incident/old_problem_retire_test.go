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

// restoredOld restores, into a manager with the old-problem rule, the
// incident a manager without it opened for finding s, and raises s
// again as the detectors do after a restart.
func restoredOld(t *testing.T, s detection.Finding) *rig {
	t.Helper()
	src := newRig(t, Config{})
	src.raise(at(0), s)
	src.tick(at(DefaultSettle))
	require.Len(t, src.m.Export(), 1)

	dst := newRig(t, Config{OldProblem: horizon})
	dst.m.Restore(src.m.Export(), at(10*time.Minute))
	dst.raise(at(time.Minute), s)
	return dst
}

// An incident restored from before the rule, made only of an old low
// problem, leaves the issues for the old problems, with no message.
func TestRestoredOldIncidentMovesToOldProblems(t *testing.T) {
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	r := restoredOld(t, old)

	p := r.only()
	assert.Equal(t, Resolved, p.State)
	assert.Empty(t, p.Members)
	assert.Equal(t, "resolved: "+string(ReasonOldProblem),
		p.Timeline[len(p.Timeline)-1].Text)
	got := r.m.OldProblems()
	require.Len(t, got, 1)
	assert.Equal(t, old.Entity, got[0].Entity)
	assert.Equal(t, []string{p.ID}, r.m.TakeQuietResolves())
	assert.True(t, r.m.ResolvedQuietly(p.ID))
	wantNone(t, r.tick(at(2*time.Minute)))
	wantNone(t, r.tick(at(time.Hour)))

	// Seen again, unchanged, it stays where it is.
	r.apply(at(2*time.Hour), detection.Changed, old)
	assert.Len(t, r.m.OldProblems(), 1)
	assert.Equal(t, Resolved, r.only().State)
}

// A problem that was young when its incident opened stays an issue,
// however long it has lasted since.
func TestRestoredRecentIncidentStays(t *testing.T) {
	young := oldSig("report", reasons.JobFailed, detection.Info,
		24*time.Hour)
	r := restoredOld(t, young)
	r.apply(at(60*24*time.Hour), detection.Changed, young)

	assert.NotEqual(t, Resolved, r.only().State)
	assert.Empty(t, r.m.OldProblems())
}

// An old problem that gets worse after it moved is a normal issue
// again, and is told.
func TestMovedOldProblemThatWorsensIsAnnounced(t *testing.T) {
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	r := restoredOld(t, old)
	require.Len(t, r.m.OldProblems(), 1)

	worse := old
	worse.Severity = detection.Warning
	r.apply(at(time.Hour), detection.Changed, worse)
	assert.Empty(t, r.m.OldProblems())
	p := r.of(old.Entity)
	assert.NotEqual(t, Resolved, p.State)
	assert.Equal(t, Notify, p.Tier)

	var told bool
	for _, d := range r.tick(at(time.Hour + DefaultSettle)) {
		told = told || d.Action == Announce && d.Incident.ID == p.ID
	}
	assert.True(t, told, "the worse problem is announced")
}

// An object is listed once: an old problem of an object that is the
// root of a live issue is not also counted with the old problems.
func TestOldProblemOfAnObjectWithAnIssueIsListedOnce(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	secret := entity(kube.KindSecret, "tls")
	old := sig(secret, reasons.TLSCertExpired, detection.Info)
	old.Since = t0.Add(-600 * 24 * time.Hour)
	r.raise(at(0), old)
	require.Len(t, r.m.OldProblems(), 1)

	pod := podSig("web")
	r.cause(pod.Entity, secret, "its certificate expired")
	r.raise(at(time.Minute), pod)

	assert.Equal(t, secret, r.only().Root)
	assert.NotEqual(t, Resolved, r.only().State)
	assert.Empty(t, r.m.OldProblems())
}

// A live incident with one young failure beside the old one is not
// moved: not every failure in it is old.
func TestIncidentWithAYoungFailureStays(t *testing.T) {
	src := newRig(t, Config{})
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	young := sig(old.Entity, reasons.JobBackoffLimitExceeded, detection.Info)
	src.raise(at(0), old, young)
	dst := newRig(t, Config{OldProblem: horizon})
	dst.m.Restore(src.m.Export(), at(10*time.Minute))
	dst.raise(at(time.Minute), old, young)

	assert.NotEqual(t, Resolved, dst.only().State)
	assert.Empty(t, dst.m.OldProblems())
}
