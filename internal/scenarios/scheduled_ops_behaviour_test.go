package scenarios

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

// schedPlain is a day of the plain schedule.
var schedPlain = schedDay{stayDown: -1}

// schedSent is one message of a replay with the time it was sent.
type schedSent struct {
	at  time.Time
	msg notification.Message
}

// schedSpan is a stretch of simulated time, [from, to).
type schedSpan struct{ from, to time.Time }

// sentBetween lists the messages sent in span.
func sentBetween(result replay.Result, span schedSpan) []schedSent {
	var out []schedSent
	for i, m := range result.Messages {
		at := result.Times[i]
		if !at.Before(span.from) && at.Before(span.to) {
			out = append(out, schedSent{at, m})
		}
	}
	return out
}

// schedEvening is the evening of day (0 is schedStart's day) until the
// next morning's scale-up; schedMorning that morning until noon.
func schedEvening(day int) schedSpan {
	from := schedStart.AddDate(0, 0, day).Add(6 * time.Hour)
	return schedSpan{from, from.Add(12*time.Hour + 45*time.Minute)}
}

func schedMorning(day int) schedSpan {
	from := schedStart.AddDate(0, 0, day).Add(-5*time.Hour - 15*time.Minute)
	return schedSpan{from, from.Add(5*time.Hour + 15*time.Minute)}
}

func describeSent(sent []schedSent) string {
	var b strings.Builder
	for _, s := range sent {
		b.WriteString("\n  " + s.at.Format(time.DateTime) + " " +
			s.msg.Title)
	}
	return b.String()
}

// A cluster put to sleep every evening at 18:00 and woken at 06:45.
// The first evening is news as before: its nodes stop reporting. From
// the third day the schedule is known from kwatch's own history: each
// evening and morning is one digest line, and the nodes the schedule
// switches off are told nothing of.
func TestScheduledOperationsCostOneLineEach(t *testing.T) {
	s := schedDays([]schedDay{schedPlain, schedPlain, schedPlain})
	result := replayLog(t, s.c.log(), replay.Options{Tail: 3 * time.Hour})

	first := sentBetween(result, schedEvening(0))
	require.NotEmpty(t, first, "the first evening is news as before")
	assert.Contains(t, first[0].msg.Title, "have not reported")
	for _, day := range []int{2, 3} {
		evening := sentBetween(result, schedEvening(day))
		require.Len(t, evening, 1, "day %d evening:%s", day+1,
			describeSent(evening))
		assert.Contains(t, evening[0].msg.Note, "Scheduled scale-down "+
			"at 18:00 UTC (seen at this time on "+strconv.Itoa(day)+
			" of the last 3 days): 8 workloads, 3 nodes.")
		assert.Equal(t, notification.StatusLow, evening[0].msg.Status)
	}
	morning := sentBetween(result, schedMorning(3))
	require.Len(t, morning, 1, "day 4 morning:%s", describeSent(morning))
	assert.Contains(t, morning[0].msg.Note, "Scheduled scale-up at "+
		"06:45 UTC (seen at this time on 2 of the last 3 days): "+
		"8 workloads, 3 nodes.")
}

// On the fourth day one workload crashes for a cause of its own, and
// the morning scale-up leaves one routed workload at zero. Both are
// told as usual: the schedule explains neither.
func TestScheduledOperationsStillTellWhatTheyDoNotExplain(t *testing.T) {
	s := schedDays([]schedDay{schedPlain, schedPlain,
		{stayDown: 1, crash: true}})
	result := replayLog(t, s.c.log(), replay.Options{Tail: 3 * time.Hour})

	var crash, left *schedSent
	for _, sent := range sentBetween(result, schedMorning(3)) {
		switch {
		case strings.Contains(sent.msg.Title, "reporter in ops"):
			crash = &sent
		case strings.Contains(sent.msg.Title, "app1 in shop"):
			left = &sent
		}
	}
	require.NotNil(t, crash, "the crash was not told")
	assert.Contains(t, crash.msg.Title, "keeps crashing")
	require.NotNil(t, left, "the workload left at zero was not told")
	assert.Contains(t, left.msg.Title, "is scaled to 0")
	scaledUp := schedMorning(3).from.Add(2 * time.Minute)
	assert.False(t, left.at.Before(scaledUp.Add(
		kube.WakeQuiet+kube.LeftBehindGrace)),
		"told at %s, before the grace after the scale-up", left.at)
	evening := sentBetween(result, schedEvening(3))
	assert.Len(t, evening, 2, "the scheduled line and the left-behind "+
		"workload's daily change:%s", describeSent(evening))
}

// On the third evening one node the schedule switches off is not
// removed. The schedule explains it until the next morning's scale-up
// is over; still not reporting a grace after that, it is told, with
// how long it has really not reported.
func TestScheduledScaleDownToldWhatDidNotComeBack(t *testing.T) {
	s := schedDays([]schedDay{schedPlain, {stayDown: -1, keepNode: true},
		schedPlain})
	result := replayLog(t, s.c.log(), replay.Options{Tail: 3 * time.Hour})

	evening := sentBetween(result, schedEvening(2))
	require.Len(t, evening, 1, "day 3 evening:%s", describeSent(evening))
	assert.Contains(t, evening[0].msg.Note, "Scheduled scale-down")
	var told *schedSent
	for _, sent := range sentBetween(result, schedMorning(3)) {
		if strings.Contains(sent.msg.Title, "pool-d2-0") {
			told = &sent
			break
		}
	}
	require.NotNil(t, told, "the node that stayed was not told")
	scaledUp := schedMorning(3).from.Add(time.Minute)
	assert.False(t, told.at.Before(scaledUp.Add(
		kube.WakeQuiet+kube.LeftBehindGrace)),
		"told at %s, before the grace after the scale-up", told.at)
	// Held overnight, it is told with its age at the time it is sent,
	// not the two minutes it had when the scale-down began.
	assert.Contains(t, told.msg.Title, "has not reported for 13 hours")
}

// kwatch restarts in the night before the fourth day. The history of
// the operations it saw is kept with its state, so the fourth evening
// is still known as scheduled: one line, and nothing about its nodes.
func TestScheduledOperationsSurviveARestart(t *testing.T) {
	store := newPersistingStore()
	s := schedDays([]schedDay{schedPlain, schedPlain})
	first := s.c.log()
	restartReplay(t, first, store, replay.Options{SyncAt: first.Start,
		Tail: time.Hour})
	s.restartAt(5, 0)
	s.morning(schedPlain)
	s.evening(schedPlain)
	second := s.c.log()
	second.Start = s.c.start
	result := restartReplay(t, second, store, replay.Options{
		SyncAt: second.Start, Tail: 3 * time.Hour})

	evening := sentBetween(result, schedEvening(3))
	require.Len(t, evening, 1, "day 4 evening:%s", describeSent(evening))
	assert.Contains(t, evening[0].msg.Note, "Scheduled scale-down at "+
		"18:00 UTC (seen at this time on 3 of the last 3 days)")
}
