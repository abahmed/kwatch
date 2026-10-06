package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// An investigator kind that keeps ignoring its budget leaves at most
// maxAbandonedPerKind goroutines behind; after that it gets no new jobs
// until one of them returns.
func TestInvestigationAbandonedGoroutinesAreBounded(t *testing.T) {
	stuck := make(chan struct{})
	e := guardedEngine(t, guardInvestigator{budget: 10 * time.Millisecond,
		fn: func(context.Context) testResult {
			<-stuck
			return testResult{}
		}})
	submit := func(id string) bool {
		return e.announcer.investigate(incident.Incident{ID: id},
			e.deps.Clock.Now())
	}

	for i := range maxAbandonedPerKind {
		require.True(t, submit(string(rune('a'+i))))
		receive(t, e)
		e.announcer.pool.received()
	}
	assert.Equal(t, int64(maxAbandonedPerKind),
		e.Stats().InvestigationsAbandoned)
	assert.True(t, e.announcer.pool.atAbandonLimit("func"))

	assert.False(t, submit("z"), "the kind is at its limit")
	assert.Equal(t, int64(1), e.Stats().InvestigationsRefused)

	close(stuck)
	require.Eventually(t, func() bool {
		return !e.announcer.pool.atAbandonLimit("func")
	}, 5*time.Second, time.Millisecond, "returned goroutines are forgotten")
	assert.True(t, submit("y"))
}
