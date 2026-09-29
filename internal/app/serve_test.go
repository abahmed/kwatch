package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
)

func shutdownDeps(
	ctx context.Context, cancel context.CancelFunc,
) *serverDeps {
	return &serverDeps{
		ctx:    ctx,
		cancel: cancel,
		deliveryManager: delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}},
		),
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{},
		),
	}
}

func TestWaitShutdownReturnsFailureForComponentError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deps := shutdownDeps(ctx, cancel)
	supervisor := newComponentSupervisor(time.Now)
	supervisor.errCh <- errors.New("core failed")

	if got := waitShutdown(deps, supervisor); got != 1 {
		t.Fatalf("waitShutdown returned %d, want 1", got)
	}
	if ctx.Err() == nil {
		t.Fatal("shutdown did not cancel the application context")
	}
}

func TestWaitShutdownReleasesLeaseAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	deps := shutdownDeps(ctx, cancel)
	released := false
	deps.setLeaseRelease(func(context.Context) { released = true })
	cancel()

	if got := waitShutdown(deps, newComponentSupervisor(time.Now)); got != 0 {
		t.Fatalf("waitShutdown returned %d, want 0", got)
	}
	if !released {
		t.Fatal("lease was not released after a graceful shutdown")
	}
}
