package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// A collecting step that panics (a digest, a listing) takes nothing: the
// decisions of the tick go on to the next step, so a page or an update
// is never lost with it. Before, the panic was caught at the boundary of
// the whole announce step and every decision of the tick was dropped.
func TestCollectorPanicLeavesTheDecisionsForTheNextStep(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0),
		ch: make(chan time.Time)}
	engine := newTestEngine(t, clock,
		func(context.Context, incident.Decision, notification.Message) {},
		nil)
	decisions := []incident.Decision{
		{Action: incident.Update, Incident: incident.Incident{ID: "a"}},
		{Action: incident.Update, Incident: incident.Incident{ID: "b"}},
	}
	before := metrics.DefaultRegistry().PanicCount("announce")

	rest, did := engine.announcer.collected(context.Background(),
		clock.now, "test", decisions,
		func(context.Context, time.Time, []incident.Decision,
		) ([]incident.Decision, bool) {
			panic("digest bug")
		})

	assert.Equal(t, decisions, rest, "nothing was taken")
	assert.False(t, did)
	assert.Equal(t, before+1,
		metrics.DefaultRegistry().PanicCount("announce"))
}

// A step that finishes hands on what it left.
func TestCollectorThatFinishesHandsOnWhatItLeft(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0),
		ch: make(chan time.Time)}
	engine := newTestEngine(t, clock,
		func(context.Context, incident.Decision, notification.Message) {},
		nil)
	decisions := []incident.Decision{{Incident: incident.Incident{ID: "a"}},
		{Incident: incident.Incident{ID: "b"}}}

	rest, did := engine.announcer.collected(context.Background(),
		clock.now, "test", decisions,
		func(_ context.Context, _ time.Time, ds []incident.Decision,
		) ([]incident.Decision, bool) {
			return ds[1:], true
		})

	assert.Equal(t, decisions[1:], rest)
	assert.True(t, did)
}
