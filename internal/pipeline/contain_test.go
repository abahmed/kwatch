package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestContainCountsPanicAndReportsSkippedStep(t *testing.T) {
	before := metrics.DefaultRegistry().PanicCount("incident")

	ok := contain("incident", "test", func() { panic("bad area") })

	if ok {
		t.Fatal("a panicking step must report as skipped")
	}
	if got := metrics.DefaultRegistry().PanicCount("incident"); got !=
		before+1 {
		t.Fatalf("panic counter = %d, want %d", got, before+1)
	}
	if !contain("incident", "test", func() {}) {
		t.Fatal("a clean step must report as finished")
	}
}

func TestAnnouncerDeliverSurvivesPanickingMessage(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0),
		ch: make(chan time.Time)}
	var sent []string
	calls := 0
	engine := newTestEngine(t, clock,
		func(_ context.Context, d incident.Decision,
			_ notification.Message,
		) {
			calls++
			if calls == 1 {
				panic("writer bug")
			}
			sent = append(sent, d.Incident.ID)
		}, nil)
	before := metrics.DefaultRegistry().PanicCount("compose")

	engine.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{
			{Action: incident.Update, Incident: incident.Incident{ID: "a"}},
			{Action: incident.Update, Incident: incident.Incident{ID: "b"}},
		})

	if len(sent) != 1 || sent[0] != "b" {
		t.Fatalf("later decisions must still be delivered, got %v", sent)
	}
	if got := metrics.DefaultRegistry().PanicCount("compose"); got !=
		before+1 {
		t.Fatalf("compose panics = %d, want %d", got, before+1)
	}
}
