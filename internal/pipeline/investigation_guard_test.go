package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// guardInvestigator plans one investigation that runs fn.
type guardInvestigator struct {
	budget time.Duration
	fn     func(context.Context) testResult
}

func (f guardInvestigator) Plan(incident.Incident) (testPlan, bool) {
	return testPlan{Kind: "func", Budget: f.budget, Run: f.fn}, true
}

func guardedEngine(t *testing.T, inv guardInvestigator) *Engine {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Investigator = inv
	})
	e.announcer.pool.grace = 20 * time.Millisecond
	startPool(t, e)
	return e
}

// A panic inside an investigator must not take the process down: the
// result is dropped (empty) and the loop still counts the job finished.
func TestInvestigationPanicIsContainedAndDropped(t *testing.T) {
	e := guardedEngine(t, guardInvestigator{
		fn: func(context.Context) testResult { panic("boom") }})

	require.True(t, e.announcer.investigate(incident.Incident{ID: "a"},
		e.deps.Clock.Now()))
	r := receive(t, e)

	assert.Equal(t, "a", r.id)
	assert.Empty(t, r.result.Evidence)
	assert.Empty(t, r.result.Output)
}

// An investigator that ignores its context cannot pin a worker past its
// budget: the pool hands back an empty result and takes the next job.
func TestInvestigationBudgetHoldsForAnInvestigatorThatIgnoresIt(t *testing.T) {
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	e := guardedEngine(t, guardInvestigator{budget: 20 * time.Millisecond,
		fn: func(context.Context) testResult {
			<-stuck
			return testResult{Output: []string{"late"}}
		}})

	for _, id := range []string{"a", "b", "c", "d"} {
		require.True(t, e.announcer.investigate(incident.Incident{ID: id},
			e.deps.Clock.Now()))
	}
	// More jobs than workers all finish: none of the stuck ones holds a
	// worker for ever.
	for range 4 {
		r := receive(t, e)
		assert.Empty(t, r.result.Output)
	}
	assert.Equal(t, int64(4), int64(e.Stats().InvestigationTimeouts))
}
