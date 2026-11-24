package scenarios

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// The scheduled-operations replay is a week of a cluster put to sleep
// every evening and woken every morning by a schedule (schedDays in
// library_scheduled_ops_test.go): every evening its apps scale to zero
// and its pool nodes are switched off and removed; every morning new
// nodes join, the apps scale up and fail their readiness probes for a
// few minutes. One morning an app is left at zero and another app
// crash-loops with a cause of its own, so the week holds one real
// problem. The noise budget gate bounds the chat messages of its
// busiest day (scorecard.GoalScheduledOpsMessagesPerDay).
var scheduledOpsPlan = []schedDay{
	{stayDown: -1}, {stayDown: -1}, {stayDown: 5, crash: true},
	{stayDown: -1}, {stayDown: -1}, {stayDown: -1},
}

// scheduledOpsTail is how long the replay runs after the last evening.
const scheduledOpsTail = 3 * time.Hour

// scheduledOpsResult is the chat volume of the scheduled-operations
// replay.
type scheduledOpsResult struct {
	messages int
	// days are the calendar days the replay spans.
	days int
	// busiest is the calendar day with the most messages, and count
	// how many it had.
	busiest string
	count   int
}

func runScheduledOps(t *testing.T) scheduledOpsResult {
	t.Helper()
	log := schedDays(scheduledOpsPlan).c.log()
	result := replayLog(t, log, replay.Options{Tail: scheduledOpsTail})
	return scheduledOpsOf(result.Times, log.Start, result.End)
}

// scheduledOpsOf counts the messages delivered at times in a replay
// from start to end.
func scheduledOpsOf(
	times []time.Time, start, end time.Time,
) scheduledOpsResult {
	r := scheduledOpsResult{messages: len(times)}
	r.busiest, r.count = scorecard.BusiestDay(times)
	first := start.UTC().Truncate(24 * time.Hour)
	r.days = int(end.UTC().Sub(first)/(24*time.Hour)) + 1
	return r
}

// scheduledOpsGate bounds the messages of the busiest day of the
// scheduled-operations replay, and shows the headroom left.
func scheduledOpsGate(r scheduledOpsResult) scorecard.Gate {
	gate := scorecard.WithHeadroom(scorecard.AtMost(
		"Messages per day (scheduled-operations replay)",
		float64(r.count), scorecard.GoalScheduledOpsMessagesPerDay, ""),
		float64(r.count), scorecard.GoalScheduledOpsMessagesPerDay)
	gate.Value += fmt.Sprintf("; busiest day %s, %d messages in %d days",
		orDash(r.busiest), r.messages, r.days)
	return gate
}

// The gate fails once the busiest day of the replay sends more than the
// budget, and passes at the budget.
func TestScheduledOpsGateFailsOverBudget(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	at := func(n int) []time.Time {
		out := make([]time.Time, n)
		for i := range out {
			out[i] = start.Add(time.Duration(i) * time.Minute)
		}
		return out
	}
	end := start.Add(50 * time.Hour)
	budget := scorecard.GoalScheduledOpsMessagesPerDay
	within := scheduledOpsGate(scheduledOpsOf(at(budget), start, end))
	over := scheduledOpsGate(scheduledOpsOf(at(budget+1), start, end))
	assert.True(t, within.Pass, within.Value)
	assert.False(t, over.Pass, over.Value)
	assert.Contains(t, over.Value, "busiest day 2026-10-05")
	assert.Contains(t, over.Value, "in 3 days")
}
