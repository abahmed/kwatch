package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// cleanup is the finalizer of a removed controller in namespace shop.
var cleanup = inventory.CoreID(explain.KindFinalizer, "shop",
	"runs.example.com/cleanup")

// stuckPeer is an object of shop stuck deleting on cleanup since age
// before t0, Low once a day has passed as the detector words it.
func stuckPeer(r *rig, name string, age time.Duration) detection.Finding {
	s := sig(entity(kube.KindSecret, name), reasons.StuckDeleting,
		detection.Info)
	if age < 24*time.Hour {
		s.Severity = detection.Warning
	}
	s.Mode = detection.ModeStuckDeleting
	s.Since = t0.Add(-age)
	s.Evidence = []detection.Evidence{{
		Label: detection.EvidenceFinalizers, Value: cleanup.Name}}
	r.cause(s.Entity, cleanup, "nothing removes the finalizer")
	return s
}

// After a restart, an object stuck since this morning on the finalizer
// of a controller removed weeks ago joins the old Low group of its
// peers: the group stays Low and nothing is sent, neither a new
// message nor a material change.
func TestYoungStuckPeerKeepsOldGroupLowAfterRestart(t *testing.T) {
	const old = 40 * 24 * time.Hour
	src := newRig(t, Config{OldProblem: horizon})
	peers := []detection.Finding{stuckPeer(src, "a", old),
		stuckPeer(src, "b", old)}
	src.raise(at(0), peers...)
	src.tick(at(DefaultSettle))
	require.Equal(t, Digest, src.only().Tier)

	dst := newRig(t, Config{OldProblem: horizon})
	start := at(time.Hour)
	dst.m.Restore(src.m.Export(), start.Add(10*time.Minute))
	young := stuckPeer(dst, "c", 3*time.Hour)
	for _, s := range peers {
		dst.cause(s.Entity, cleanup, "nothing removes the finalizer")
	}
	dst.raise(start, append(peers, young)...)

	for d := time.Duration(0); d <= 2*time.Hour; d += time.Minute {
		wantNone(t, dst.tick(start.Add(d)))
	}
	p := dst.only()
	assert.Contains(t, p.Members, young.Key())
	assert.Equal(t, Digest, p.Tier)
}

// A live failure that joins the same old group is news: the group is
// Needs attention again.
func TestLiveFailureLiftsOldLeftoverGroup(t *testing.T) {
	const old = 40 * 24 * time.Hour
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), stuckPeer(r, "a", old), stuckPeer(r, "b", old))
	require.Equal(t, Digest, r.only().Tier)

	live := sig(entity(kube.KindPod, "cleaner"), reasons.CrashLoopBackOff,
		detection.Warning)
	live.Since = at(time.Minute)
	r.cause(live.Entity, cleanup, "nothing removes the finalizer")
	r.raise(at(time.Minute), live)

	assert.Equal(t, Notify, r.of(cleanup).Tier)
}

// A stuck deletion that flips from Low back to Warning did not get
// worse: an old group it belongs to stays Low.
func TestLeftoverFlipToWarningIsNotARise(t *testing.T) {
	const old = 40 * 24 * time.Hour
	r := newRig(t, Config{OldProblem: horizon})
	a, b := stuckPeer(r, "a", old), stuckPeer(r, "b", old)
	r.raise(at(0), a, b)

	a.Severity = detection.Warning
	r.apply(at(time.Minute), detection.Changed, a)

	p := r.only()
	assert.Equal(t, Digest, p.Tier)
	assert.False(t, p.marks.Rose)
}

// Without a restart too: a young stuck peer joining an announced old
// Low group sends no material change.
func TestYoungStuckPeerSendsNoMaterialChange(t *testing.T) {
	const old = 40 * 24 * time.Hour
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), stuckPeer(r, "a", old), stuckPeer(r, "b", old))
	r.tick(at(DefaultSettle))
	young := stuckPeer(r, "c", 3*time.Hour)
	r.raise(at(time.Hour), young)

	for d := time.Hour; d <= 3*time.Hour; d += time.Minute {
		wantNone(t, r.tick(at(d)))
	}
	assert.Equal(t, Digest, r.only().Tier)
}
