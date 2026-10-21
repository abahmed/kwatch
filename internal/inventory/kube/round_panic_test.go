package kube

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// brokenModel panics on every read: its embedded Reader is nil.
type brokenModel struct{ inventory.Reader }

func runsAndStops(t *testing.T, run func(context.Context)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not stop with its context")
	}
}

// A panic in one round must not end the process: the loop loses that
// round, then stops normally with its context.
func TestCrashLogRoundRunSurvivesAPanickingRound(t *testing.T) {
	round := NewCrashLogRound(CrashLogConfig{
		Model:  brokenModel{},
		Now:    time.Now,
		Submit: func(context.Context, ...inventory.Observation) {},
	})

	runsAndStops(t, round.Run)
}

func TestProberRunSurvivesAPanickingRound(t *testing.T) {
	prober := NewProber(ProbeConfig{
		Now:    time.Now,
		Submit: func(context.Context, ...inventory.Observation) {},
	}) // no client: the first probe panics

	runsAndStops(t, prober.Run)
}
