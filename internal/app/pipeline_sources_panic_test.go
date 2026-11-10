package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A panicking runner is contained and fails the pipeline run, so the
// supervisor restarts it instead of leaving a dead input.
func TestStartSourcesFailsThePipelineOnAPanickingRunner(t *testing.T) {
	ctx, fail := context.WithCancelCause(context.Background())
	defer fail(nil)
	ran := make(chan struct{})
	stop := startSources(ctx, []func(context.Context){
		func(context.Context) { panic("boom") },
		func(ctx context.Context) {
			close(ran)
			<-ctx.Done()
		},
	}, fail)

	<-ran
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
	if cause := context.Cause(ctx); !errors.Is(cause, errSourcePanicked) {
		t.Fatalf("cause = %v, want errSourcePanicked", cause)
	}
	stop()
}

// The pipeline run itself returns the error, so the supervisor sees it.
func TestRunEngineReturnsAnErrorWhenASourcePanics(t *testing.T) {
	err := runEngine(context.Background(),
		[]func(context.Context){
			func(context.Context) { panic("boom") },
		},
		func(ctx context.Context) error {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
			return nil
		})
	if !errors.Is(err, errSourcePanicked) {
		t.Fatalf("run error = %v, want errSourcePanicked", err)
	}
}
