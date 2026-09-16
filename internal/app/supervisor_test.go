package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
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
	healthServer := health.NewHealthServer(config.HealthCheck{})
	initialized := make(chan struct{})
	close(initialized)
	started := make(chan struct{})
	supervisor.startOptional(
		context.Background(), initialized,
		componentSpec{
			name: "optional-test-component",
			run: func(context.Context) error {
				close(started)
				return errors.New("optional failure")
			},
		},
		healthServer,
	)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("optional component did not start")
	}
	supervisor.wg.Wait()

	got := healthServer.ComponentErrors()["optional-test-component"]
	if got != "component_failed" {
		t.Fatalf("optional failure reason = %q", got)
	}
	select {
	case err := <-supervisor.errCh:
		t.Fatalf("optional failure was reported as fatal: %v", err)
	default:
	}
}
