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
)

// nodeDown is a finding that pages: a node stopped being ready.
func nodeDown() detection.Finding {
	return sig(inventory.CoreID(kube.KindNode, "", "n1"),
		reasons.NodeNotReady, detection.Critical)
}

// tickEvery ticks r every 10 seconds from from to to and returns every
// decision.
func tickEvery(r *rig, from, to time.Duration) []Decision {
	var out []Decision
	for now := from; now <= to; now += 10 * time.Second {
		out = append(out, r.tick(at(now))...)
	}
	return out
}

func pagedRig(t *testing.T) (*rig, time.Time) {
	t.Helper()
	r := newRig(t, Config{})
	r.raise(at(0), nodeDown())
	ds := tickEvery(r, 0, DefaultSettle)
	require.Len(t, ds, 1)
	require.Equal(t, Page, ds[0].Incident.Tier)
	return r, r.only().Announced
}

func TestPageGetsASixHourReminderThenWeekly(t *testing.T) {
	r, start := pagedRig(t)

	wantNone(t, r.tick(start.Add(PageRemindAfter-time.Second)))
	first := start.Add(PageRemindAfter)
	ds := r.tick(first)
	wantAction(t, ds, Update, ReasonReminder)
	assert.Equal(t, Page, ds[0].Incident.Tier,
		"the reminder is an update, not a new page")

	wantNone(t, r.tick(first.Add(PageRemindAfter)))
	wantNone(t, r.tick(first.Add(RemindEvery-time.Second)))
	wantAction(t, r.tick(first.Add(RemindEvery)), Update, ReminderReasonForTest)
}

// ReminderReasonForTest keeps the weekly assertion readable.
const ReminderReasonForTest = ReasonReminder

func TestPageReminderWakesTheTick(t *testing.T) {
	r, start := pagedRig(t)
	_, next := r.m.Tick(start.Add(time.Hour))
	assert.Equal(t, PageRemindAfter-time.Hour, next)
}

// resolvePage clears the node and runs until the incident resolves.
func resolvePage(t *testing.T, r *rig, clearAt time.Duration) time.Duration {
	t.Helper()
	r.clear(at(clearAt), nodeDown())
	for now := clearAt; now <= clearAt+time.Hour; now += 10 * time.Second {
		for _, d := range r.tick(at(now)) {
			if d.Action == Resolve {
				return now
			}
		}
	}
	t.Fatal("page never resolved")
	return 0
}

func TestPageOutsideRepageWindowPagesAgain(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + RepageWindow + time.Minute
	r.raise(at(raiseAt), nodeDown())
	ds := tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)
	require.NotEmpty(t, ds)
	assert.Equal(t, Page, ds[0].Incident.Tier)
}

func TestHeldRepeatKeepsPageReminder(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + 10*time.Minute
	r.raise(at(raiseAt), nodeDown())
	tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)
	start := r.only().Announced
	wantAction(t, r.tick(start.Add(PageRemindAfter)), Update, ReasonReminder)
}

// replicaRig wires four pods of one deployment and returns a function
// that makes the first n of them crash.
func replicaRig(t *testing.T) (*rig, func(at time.Time, n int)) {
	t.Helper()
	r := newRig(t, Config{})
	var pods []inventory.EntityID
	for i := 0; i < 4; i++ {
		pod := entity(kube.KindPod, "web-"+string(rune('a'+i)))
		r.relate(pod, inventory.OwnedBy, entity(kube.KindReplicaSet, "web-rs"))
		pods = append(pods, pod)
	}
	r.relate(entity(kube.KindReplicaSet, "web-rs"), inventory.OwnedBy,
		entity(kube.KindDeployment, "web"))
	return r, func(now time.Time, n int) {
		for _, pod := range pods[:n] {
			r.apply(now, detection.Raised,
				sig(pod, reasons.CrashLoopBackOff, detection.Warning))
		}
	}
}

func TestRolledUpIncidentUpdatesWhenMorePodsFail(t *testing.T) {
	r, crash := replicaRig(t)
	crash(at(0), 1)
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	r.m.RecordRolledUp(ds[0].Incident.ID)

	// Same pod count: nothing to say.
	wantNone(t, r.tick(at(DefaultSettle+time.Minute)))
	// Doubling the failing pods is news.
	crash(at(2*time.Minute), 4)
	wantAction(t, r.tick(at(DefaultSettle+2*time.Minute)), Update,
		ReasonMaterialChange)
	// And so is not news again at the same size.
	wantNone(t, r.tick(at(DefaultSettle+3*time.Minute)))
}

func TestPodGrowthIsIgnoredWithoutRollup(t *testing.T) {
	r, crash := replicaRig(t)
	crash(at(0), 1)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	crash(at(2*time.Minute), 4)
	wantNone(t, r.tick(at(DefaultSettle+2*time.Minute)))
}

func TestRolledUpIncidentIsRemindedDaily(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	r.m.RecordRolledUp(ds[0].Incident.ID)
	start := r.only().Announced

	wantNone(t, r.tick(start.Add(ChronicRemindEvery-time.Second)))
	wantAction(t, r.tick(start.Add(ChronicRemindEvery)), Update, ReasonReminder)
}

func TestDigestTierIncidentIsRemindedDaily(t *testing.T) {
	r := newRig(t, Config{})
	hpa := sig(entity(kube.KindPod, "web"), reasons.ServiceUnused,
		detection.Warning)
	r.raise(at(0), hpa)
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	require.Equal(t, Digest, ds[0].Incident.Tier)
	start := r.only().Announced
	wantAction(t, r.tick(start.Add(ChronicRemindEvery)), Update, ReasonReminder)
}

func TestPagedFlagTracksAnnouncementsAndSurvivesRestore(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	id := ds[0].Incident.ID
	assert.False(t, r.m.Paged(id))
	r.m.RecordPaged(id, true)
	assert.True(t, r.m.Paged(id))
	assert.True(t, r.m.Paged("unknown"), "an unknown incident resolves normally")

	dst := newRig(t, Config{})
	dst.m.Restore(r.m.Export(), time.Time{})
	assert.True(t, dst.m.Paged(id))

	// An old record without the flag restores announced incidents as paged.
	old := r.m.Export()
	for i := range old {
		old[i].Paged, old[i].PagedKnown = false, false
	}
	legacy := newRig(t, Config{})
	legacy.m.Restore(old, time.Time{})
	assert.True(t, legacy.m.Paged(id))
}

func TestDigestWorthy(t *testing.T) {
	now := at(48 * time.Hour)
	rhythm := []time.Time{now.Add(-3 * time.Hour), now.Add(-2 * time.Hour),
		now.Add(-time.Hour), now}
	tests := []struct {
		name string
		p    Incident
		want bool
	}{
		{"first occurrence is a blip", Incident{}, false},
		{"recurrence is reported", Incident{
			History: []Occurrence{{Heard: true}}}, true},
		{"rhythm listed lately stays quiet", Incident{
			Occurrences: rhythm, DigestedAt: now.Add(-time.Hour),
			History: []Occurrence{{}}}, false},
		{"rhythm listed long ago is reported", Incident{
			Occurrences: rhythm, DigestedAt: now.Add(-25 * time.Hour),
			History: []Occurrence{{}}}, true},
		{"rhythm never listed is reported", Incident{
			Occurrences: rhythm, History: []Occurrence{{}}}, true},
		{"rhythm listed by an earlier occurrence", Incident{
			Occurrences: rhythm, History: []Occurrence{
				{Digested: now.Add(-time.Hour)}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DigestWorthy(tt.p, now))
		})
	}
}

func TestRecordDigestedIsKept(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	ds := r.tick(at(DefaultSettle))
	r.m.RecordDigested(ds[0].Incident.ID, at(time.Hour))
	assert.Equal(t, at(time.Hour), r.only().DigestedAt)
}

func TestRepeatNoteWordsTheWindowInWholeHours(t *testing.T) {
	assert.Equal(t, "failing again: 4th time in 2h", repeatNote(4))
	assert.Equal(t, "90m", windowText(90*time.Minute))
}
