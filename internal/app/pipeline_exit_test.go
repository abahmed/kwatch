package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/pipeline"
)

// A fenced store after the session ended means a newer leader took
// over: the pipeline stops cleanly; every other error is kept.
func TestPipelineExitTreatsFencingAsACleanStop(t *testing.T) {
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	fenced := fmt.Errorf("run: %w", pipeline.ErrFenced)
	if got := pipelineExit(ended, fenced); !errors.Is(
		got, errComponentCleanStop) {
		t.Fatalf("fenced exit = %v, want a clean stop", got)
	}
	other := errors.New("boom")
	if got := pipelineExit(ended, other); got != other {
		t.Fatalf("other exit = %v, want it unchanged", got)
	}
	if got := pipelineExit(ended, nil); got != nil {
		t.Fatalf("nil exit = %v, want nil", got)
	}
}

// A fence while the lease is still held must fail the component, or the
// replica stays ready with no pipeline.
func TestPipelineExitFailsWhenFencedWhileLeaseHeld(t *testing.T) {
	fenced := fmt.Errorf("run: %w", pipeline.ErrFenced)
	got := pipelineExit(context.Background(), fenced)
	if got == nil || errors.Is(got, errComponentCleanStop) {
		t.Fatalf("exit = %v, want a real error", got)
	}
	if !errors.Is(got, errFencedWhileLeading) {
		t.Fatalf("exit = %v, want errFencedWhileLeading", got)
	}
}
