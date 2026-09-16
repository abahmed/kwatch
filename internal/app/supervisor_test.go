package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComponentSupervisorReportsRequiredFailure(t *testing.T) {
	supervisor := newComponentSupervisor()
	started := make(chan struct{})
	supervisor.startOwned(context.Background(), componentSpec{
		name:     "required-test-component",
		required: true,
		run: func(context.Context) error {
			close(started)
			return errors.New("startup failed")
		},
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("required component did not start")
	}
	select {
	case err := <-supervisor.errCh:
		if err == nil || err.Error() != "required-test-component: startup failed" {
			t.Fatalf("unexpected component error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("required component failure was not reported")
	}

	supervisor.wg.Wait()
}

func TestComponentSupervisorMarksOptionalFailureDegraded(t *testing.T) {
	supervisor := newComponentSupervisor()
	initialized := make(chan struct{})
	close(initialized)
	started := make(chan struct{})
	var gotError error
	supervisor.startOptional(
		context.Background(), initialized,
		componentSpec{
			name:    "optional-test-component",
			onError: func(err error) { gotError = err },
			run: func(context.Context) error {
				close(started)
				return errors.New("optional failure")
			},
		},
	)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("optional component did not start")
	}
	supervisor.wg.Wait()

	if gotError == nil || gotError.Error() != "optional failure" {
		t.Fatalf("optional failure callback = %v", gotError)
	}
	select {
	case err := <-supervisor.errCh:
		t.Fatalf("optional failure was reported as fatal: %v", err)
	default:
	}
}
