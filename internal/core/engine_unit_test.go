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
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink:      func(context.Context, problem.Decision, notice.Message) {},
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
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink:      func(context.Context, problem.Decision, notice.Message) {},
		Clock:     &fakeClock{now: time.Now()},
	}

	engine, _ := NewEngine(deps)

	done := make(chan bool)
	go func() {
		select {
		case <-engine.wake:
			done <- true
		case <-time.After(100 * time.Millisecond):
			done <- false
		}
	}()

	engine.Submit(context.Background(), knowledge.Fact{Kind: knowledge.Observed})

	if !<-done {
		t.Error("Submit did not wake engine")
	}
}

func TestSourcesSyncedSignals(t *testing.T) {
	deps := Dependencies{
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink:      func(context.Context, problem.Decision, notice.Message) {},
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
		t.Error("SourcesSynced did not signal")
	}
}
