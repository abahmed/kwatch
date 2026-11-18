package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// The engine publishes the incidents' view before its first iteration
// and again at the end of every iteration, before it reports progress:
// a reader that saw progress sees that iteration's view.
func TestEnginePublishesOneViewPerIteration(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC),
		ch: make(chan time.Time)}
	seen := make(chan uint64, 4)
	engine := newTestEngine(t, clock,
		func(context.Context, incident.Decision, notification.Message) {},
		func(d *Dependencies) {
			d.Progress = func() {
				seen <- d.Incidents.View().Revision
			}
		})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()

	clock.ch <- clock.now
	first := <-seen
	clock.ch <- clock.now
	second := <-seen
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if first < 2 || second != first+1 {
		t.Fatalf("revisions %d then %d: want one per iteration "+
			"after the first publish", first, second)
	}
}
