package core

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/notice"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestColdStartFlag(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now, ch: make(chan time.Time)}

	deps := Dependencies{
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink: func(context.Context, problem.Decision,
			notice.Message) {
		},
		Clock: clock,
	}

	engine, _ := NewEngine(deps)

	engine.coldStart = true
	if !engine.coldStart {
		t.Error("cold start flag not set")
	}
}

func TestWarmStartFlag(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now, ch: make(chan time.Time)}

	deps := Dependencies{
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink: func(context.Context, problem.Decision,
			notice.Message) {
		},
		Clock: clock,
	}

	engine, _ := NewEngine(deps)

	engine.coldStart = false
	if engine.coldStart {
		t.Error("warm start should have cold start = false")
	}
}

func TestStartupWindowTimeout(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now, ch: make(chan time.Time)}

	deps := Dependencies{
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink: func(context.Context, problem.Decision,
			notice.Message) {
		},
		Clock: clock,
	}

	engine, _ := NewEngine(deps)

	engine.coldStart = true
	engine.startupUntil = now.Add(1 * time.Minute)

	if engine.startupUntil.Before(now) {
		t.Error("startup window should be in future")
	}
}
