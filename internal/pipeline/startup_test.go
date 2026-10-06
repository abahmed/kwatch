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

func TestColdStartFlag(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now, ch: make(chan time.Time)}

	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink: func(context.Context, incident.Decision,
			notification.Message) {
		},
		Clock: clock,
	}

	engine, _ := NewEngine(deps)

	engine.announcer.collect.Startup.ColdStart = true
	if !engine.announcer.collect.Startup.ColdStart {
		t.Error("cold start flag not set")
	}
}

func TestWarmStartFlag(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now, ch: make(chan time.Time)}

	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink: func(context.Context, incident.Decision,
			notification.Message) {
		},
		Clock: clock,
	}

	engine, _ := NewEngine(deps)

	engine.announcer.collect.Startup.ColdStart = false
	if engine.announcer.collect.Startup.ColdStart {
		t.Error("warm start should have cold start = false")
	}
}

func TestStartupWindowTimeout(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now, ch: make(chan time.Time)}

	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil),
		Incidents: incident.NewManager(incident.Config{}, nil),
		Sink: func(context.Context, incident.Decision,
			notification.Message) {
		},
		Clock: clock,
	}

	engine, _ := NewEngine(deps)

	engine.announcer.collect.Startup.ColdStart = true
	engine.announcer.collect.Startup.Until = now.Add(1 * time.Minute)

	if engine.announcer.collect.Startup.Until.Before(now) {
		t.Error("startup window should be in future")
	}
}
