package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

type fakeClock struct {
	now time.Time
	ch  chan time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) After(time.Duration) <-chan time.Time {
	return f.ch
}

func TestNewEngineRejectsMissingDeps(t *testing.T) {
	_, err := NewEngine(Dependencies{})
	if err == nil {
		t.Error("expected error for nil dependencies")
	}
}

func TestNewEngineWithValidDeps(t *testing.T) {
	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink:      func(context.Context, incident.Decision, notification.Message) {},
		Clock:     &fakeClock{now: time.Now()},
	}

	engine, err := NewEngine(deps)

	if err != nil {
		t.Errorf("NewEngine failed: %v", err)
	}
	if engine == nil {
		t.Error("NewEngine returned nil")
	}
}

func TestSubmitWakesEngine(t *testing.T) {
	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink:      func(context.Context, incident.Decision, notification.Message) {},
		Clock:     &fakeClock{now: time.Now()},
	}

	engine, _ := NewEngine(deps)

	done := make(chan bool)
	go func() {
		select {
		case <-engine.inbox.wake:
			done <- true
		case <-time.After(100 * time.Millisecond):
			done <- false
		}
	}()

	engine.Submit(context.Background(),
		inventory.Observation{Kind: inventory.Observed})

	if !<-done {
		t.Error("Submit did not wake engine")
	}
}

func TestSourcesSyncedFindings(t *testing.T) {
	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink:      func(context.Context, incident.Decision, notification.Message) {},
		Clock:     &fakeClock{now: time.Now()},
	}

	engine, _ := NewEngine(deps)

	done := make(chan bool)
	go func() {
		select {
		case <-engine.synced:
			done <- true
		case <-time.After(100 * time.Millisecond):
			done <- false
		}
	}()

	engine.SourcesSynced()

	if !<-done {
		t.Error("SourcesSynced did not finding")
	}
}

func TestEngineIncidentsEmptyWithoutFindings(t *testing.T) {
	engine := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink, nil)
	if got := engine.Incidents(); len(got) != 0 {
		t.Fatalf("want no incidents, got %+v", got)
	}
}
