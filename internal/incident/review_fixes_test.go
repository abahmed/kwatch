package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A crash loop that began after the announcement was told once. After a
// restart it is raised again later than the adoption window; that must
// not be told a second time.
func TestStagePeakSurvivesARestart(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.raise(at(2*time.Minute), crashSig(pod))
	wantAction(t, r.tick(at(2*time.Minute)), Update, ReasonMaterialChange)
	recs := r.m.Export()
	require.NotZero(t, recs[0].StagePeak)

	fresh := newRig(t, Config{})
	_, pod2 := fresh.workloadRig(t, 2, 1, 1)
	fresh.m.Restore(recs, at(20*time.Minute))
	fresh.raise(at(10*time.Minute), notReadySig(pod2))
	for now := 10 * time.Minute; now <= 35*time.Minute; now += 10 * time.Second {
		wantNone(t, fresh.tick(at(now)))
	}
	fresh.raise(at(36*time.Minute), crashSig(pod2))
	wantNone(t, fresh.tick(at(36*time.Minute)))
}

// The fix attempt of an earlier occurrence is not the new occurrence's.
func TestReopenForgetsTheEarlierFixAttempt(t *testing.T) {
	r, web, failing := openedRollout(t)
	r.change(web, 3*time.Minute, rollout("15"))
	r.touch(at(3*time.Minute), web)
	wantAction(t, r.tick(at(3*time.Minute)), Update, ReasonFixAttempt)
	r.m.mu.Lock()
	r.m.lookup(web).attemptSeen = &inventory.Change{Revision: "16"}
	r.m.mu.Unlock()

	r.clear(at(5*time.Minute), failing)
	var resolvedAt time.Duration
	for now := 5 * time.Minute; now < time.Hour && resolvedAt == 0; now +=
		10 * time.Second {
		for _, d := range r.tick(at(now)) {
			if d.Action == Resolve {
				resolvedAt = now
			}
		}
	}
	require.NotZero(t, resolvedAt, "the incident resolves")

	r.raise(at(resolvedAt+time.Minute), failing)
	r.m.mu.Lock()
	p := r.m.lookup(web)
	r.m.mu.Unlock()
	assert.Nil(t, p.Attempt)
	assert.Nil(t, p.attemptSeen)
	assert.False(t, p.attemptLate)
}

// An announcement that was decided but never delivered did not page
// anyone, so it is not a page to suppress the next one with.
func TestOccurrenceCountsOnlyPagesThatReachedSomeone(t *testing.T) {
	p := &Incident{Tier: Page, Announced: at(time.Minute)}
	assert.False(t, occurrenceOf(p).Page, "never delivered, never paged")

	p.sent.told = true
	assert.True(t, occurrenceOf(p).Page, "chat heard it")

	p.sent.told = false
	p.Delivery.MarkPaged()
	assert.True(t, occurrenceOf(p).Page, "the pagers heard it")
}

// A root kind kwatch cannot observe proves no recovery, so findings
// that return for it do not count a flap cycle.
func TestRecoveringUnverifiableRootCountsNoCycle(t *testing.T) {
	sight := &kindSight{hidden: map[inventory.Kind]bool{}}
	r := newRig(t, Config{Verifiable: sight.verifiable})
	web := podSig("web")
	announced(t, r, web)
	r.clear(at(5*time.Minute), web)
	wantNone(t, r.tick(at(5*time.Minute)))
	require.Equal(t, Recovering, r.only().State)

	sight.hidden[kube.KindPod] = true
	r.raise(at(6*time.Minute), web)
	wantNone(t, r.tick(at(6*time.Minute)))

	assert.Equal(t, Open, r.only().State)
	assert.Empty(t, r.only().Cycles, "no recovery was proven")
}

// A flapping incident whose members keep swapping cause does not grow
// movedTo without bound, and one root moved to twice counts once.
func TestMovedToIsDedupedAndBounded(t *testing.T) {
	p := &Incident{}
	a := entity(kube.KindDeployment, "a")
	p.noteMoved(a)
	p.noteMoved(a)
	assert.Equal(t, []inventory.EntityID{a}, p.movedTo)
	assert.False(t, dispersed(p.movedTo))

	for i := range 20 {
		p.noteMoved(entity(kube.KindDeployment, string(rune('b'+i))))
	}
	assert.Len(t, p.movedTo, maxMoved)
	assert.True(t, dispersed(p.movedTo))
}

// The first announcement's route is recorded, persisted and survives a
// restart, so the resolve is routed as the announcement was.
func TestAnnouncedRouteIsRecordedAndPersisted(t *testing.T) {
	r, _ := pagedRig(t)
	d := r.only()
	require.NotNil(t, d.AnnouncedRoute)
	assert.Equal(t, "critical", d.AnnouncedRoute.Severity)
	assert.Contains(t, d.AnnouncedRoute.Reasons, nodeDown().Reason)

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), time.Time{})
	got := fresh.only().AnnouncedRoute
	require.NotNil(t, got)
	assert.Equal(t, d.AnnouncedRoute, got)

	got.Reasons[0] = "changed"
	assert.NotEqual(t, "changed", fresh.only().AnnouncedRoute.Reasons[0],
		"a snapshot never shares its slices")
}

// A resolve that ends at the StillBrokenMax cap, with the workload
// still short of replicas, must not claim the workload is healthy.
func TestCappedStillBrokenResolveDoesNotSayHealthy(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 0, 5)
	f := notReadySig(pod)
	r.raise(at(0), f)
	require.Len(t, r.tick(at(DefaultSettle)), 1)
	r.clear(at(5*time.Minute), f)
	r.tick(at(5 * time.Minute))

	ds := r.tick(at(5*time.Minute + StillBrokenMax + time.Minute))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
	assert.Equal(t, Reason("stopped tracking after 2h; "+
		"coverage check continues"), ds[0].Reason)
}
