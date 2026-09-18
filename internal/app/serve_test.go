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

func TestWaitShutdownReturnsFailureForControllerError(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	controllerDone := make(chan struct{})
	close(controllerDone)

	deps := &serverDeps{
		cancel:         cancel,
		controllerDone: controllerDone,
		deliveryManager: delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}},
		),
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{},
		),
		cleanup: func() {},
	}
	supervisor := newComponentSupervisor(time.Now)
	supervisor.errCh <- errors.New("cache sync failed")

	if got := waitShutdown(deps, supervisor); got != 1 {
		t.Fatalf("waitShutdown returned %d, want 1", got)
	}
}

func TestWaitShutdownReturnsWhenApplicationContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	controllerDone := make(chan struct{})
	close(controllerDone)
	deps := &serverDeps{
		ctx:            ctx,
		cancel:         cancel,
		controllerDone: controllerDone,
		deliveryManager: delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}},
		),
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{},
		),
		cleanup: func() {},
	}
	supervisor := newComponentSupervisor(time.Now)
	cancel()

	if got := waitShutdown(deps, supervisor); got != 0 {
		t.Fatalf("waitShutdown returned %d, want 0", got)
	}
}
