package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failAgain raises the node, ticks until the reopened incident speaks
// and returns the decision. resolved is when the page resolved.
func failAgain(
	t *testing.T, r *rig, resolved time.Duration, gap time.Duration,
) (Decision, time.Duration) {
	t.Helper()
	raiseAt := resolved + gap
	r.raise(at(raiseAt), nodeDown())
	ds := tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)
	require.Len(t, ds, 1)
	return ds[0], raiseAt
}

func timelineOf(p Incident) string {
	var notes []string
	for _, e := range p.Timeline {
		notes = append(notes, e.Text)
	}
	return strings.Join(notes, "\n")
}

func TestPageReopensTheSameIncident(t *testing.T) {
	r, _ := pagedRig(t)
	first := r.only()
	resolved := resolvePage(t, r, 5*time.Minute)

	var d Decision
	var raiseAt time.Duration
	for i, want := range []string{"2nd", "3rd", "4th"} {
		// Uneven gaps, so the recurrences show no rhythm.
		d, raiseAt = failAgain(t, r, resolved, time.Duration(7+i*9)*time.Minute)
		assert.Equal(t, Update, d.Action)
		assert.Equal(t, ReasonFailingAgain, d.Reason)
		assert.Equal(t, first.ID, d.Incident.ID, "same Slack thread")
		assert.Equal(t, first.AlertKey, d.Incident.AlertKey,
			"same paging alert")
		assert.Equal(t, Notify, d.Incident.Tier, "no new page")
		assert.Contains(t, timelineOf(d.Incident),
			"failing again: "+want+" time in 2h")
		assert.Equal(t, i+2, d.Incident.RepeatCount, "typed count")
		resolved = resolvePage(t, r, raiseAt+DefaultSettle+2*time.Minute)
	}
	assert.Len(t, r.m.Incidents(), 1, "no second incident was opened")
}

func TestReopenedPageResolvesUnderTheSameID(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	resolved := resolvePage(t, r, 5*time.Minute)
	_, raiseAt := failAgain(t, r, resolved, 10*time.Minute)

	r.clear(at(raiseAt+3*time.Minute), nodeDown())
	var got []Decision
	for now := raiseAt + 3*time.Minute; now < raiseAt+time.Hour; now +=
		10 * time.Second {
		got = append(got, r.tick(at(now))...)
	}
	require.Len(t, got, 1)
	assert.Equal(t, Resolve, got[0].Action)
	assert.Equal(t, id, got[0].Incident.ID)
}

func TestReopenedPageKeepsItsReminder(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	failAgain(t, r, resolved, 10*time.Minute)
	start := r.only().Announced
	wantNone(t, r.tick(start.Add(PageRemindAfter-time.Second)))
	wantAction(t, r.tick(start.Add(PageRemindAfter)), Update, ReasonReminder)
}

func TestPageAfterRepageWindowIsANewIncident(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	resolved := resolvePage(t, r, 5*time.Minute)
	d, _ := failAgain(t, r, resolved, RepageWindow+time.Minute)
	assert.Equal(t, Announce, d.Action)
	assert.Equal(t, Page, d.Incident.Tier)
	assert.NotEqual(t, id, d.Incident.ID)
}

func TestReopenKeepsTheFlapHold(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	for i := 0; i < 2; i++ {
		_, raiseAt := failAgain(t, r, resolved, time.Duration(6+i*7)*time.Minute)
		resolved = resolvePage(t, r, raiseAt+DefaultSettle+2*time.Minute)
	}
	_, raiseAt := failAgain(t, r, resolved, 8*time.Minute)
	p := r.only()
	assert.GreaterOrEqual(t, len(p.Occurrences), ChronicOccurrences)
	assert.Greater(t, r.m.holdFor(&p, at(raiseAt+time.Minute)), DefaultHold,
		"a page that keeps coming back waits longer before it resolves")
}

func TestReopenedUpdateSurvivesARestart(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + 10*time.Minute
	r.raise(at(raiseAt), nodeDown())
	require.Empty(t, r.tick(at(raiseAt+time.Second)))

	dst := newRig(t, Config{})
	dst.m.Restore(r.m.Export(), time.Time{})
	dst.raise(at(raiseAt+2*time.Second), nodeDown())
	ds := dst.tick(at(raiseAt + DefaultReviseSettle + 5*time.Second))
	wantAction(t, ds, Update, ReasonFailingAgain)
	assert.Equal(t, id, ds[0].Incident.ID)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
}

func TestResolvedPageIsReopenedAfterARestart(t *testing.T) {
	r, _ := pagedRig(t)
	first := r.only()
	resolved := resolvePage(t, r, 5*time.Minute)

	dst := newRig(t, Config{})
	dst.m.Restore(r.m.Export(), time.Time{})
	d, _ := failAgain(t, dst, resolved, 10*time.Minute)
	assert.Equal(t, ReasonFailingAgain, d.Reason)
	assert.Equal(t, first.ID, d.Incident.ID)
	assert.Equal(t, first.AlertKey, d.Incident.AlertKey)
}

func TestReopenWakesTheTick(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := at(resolved + 10*time.Minute)
	r.raise(raiseAt, nodeDown())
	_, next := r.m.Tick(raiseAt)
	assert.Equal(t, DefaultReviseSettle, next)
}

// A reopened incident that recovers before its "failing again" update is
// due still owes it: the thread's last word is the old resolve. The
// update is sent once, when the failure returns, and the incident is Open
// by then, so no "material change" follows it.
func TestReopenedUpdateIsSentOnceWhenItRecoversFirst(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + 10*time.Minute
	r.raise(at(raiseAt), nodeDown())
	require.Empty(t, r.tick(at(raiseAt+time.Second)))

	r.clear(at(raiseAt+5*time.Second), nodeDown())
	require.Empty(t, tickEvery(r, raiseAt+5*time.Second, raiseAt+time.Minute))
	back := raiseAt + 2*time.Minute
	r.raise(at(back), nodeDown())

	ds := tickEvery(r, back, back+DefaultSettle)
	wantAction(t, ds, Update, ReasonFailingAgain)
	assert.Equal(t, Open, ds[0].Incident.State)
}

// The repeat count and the member events survive a restart, so the
// "failing again" message keeps its count.
func TestRepeatCountAndMemberEventsArePersisted(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	failAgain(t, r, resolved, 10*time.Minute)

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), time.Time{})

	got := fresh.only()
	assert.Equal(t, 2, got.RepeatCount)
	var about int
	for _, e := range got.Timeline {
		if e.Entity != nil {
			about++
		}
	}
	assert.Positive(t, about, "member events keep their entity")
}

// A reopened incident that recovers for good before its "failing again"
// update was ever sent never told the thread it was back: the thread
// still ends on the first resolve. It closes again without a second
// resolve message, and stays reopenable.
func TestReopenThatRecoversUnheardResolvesNoSecondTime(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + 10*time.Minute
	r.raise(at(raiseAt), nodeDown())
	require.Empty(t, r.tick(at(raiseAt+time.Second)))

	r.clear(at(raiseAt+5*time.Second), nodeDown())
	got := tickEvery(r, raiseAt+5*time.Second, raiseAt+time.Hour)

	assert.Empty(t, got, "the thread already says resolved")
	p := r.only()
	assert.Equal(t, id, p.ID)
	assert.Equal(t, Resolved, p.State)
	assert.False(t, p.Pending.ReopenOwed())
	assert.True(t, p.CanReopen(), "a later failure still reopens it")
}
