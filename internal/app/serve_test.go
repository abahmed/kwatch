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

	var reason string
	var failureComponent, failureCode string
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
		endSession: func(_ context.Context, value string) {
			reason = value
		},
		recordFailure: func(_ context.Context, component, code string) {
			failureComponent, failureCode = component, code
		},
	}
	supervisor := newComponentSupervisor(time.Now)
	supervisor.errCh <- errors.New("cache sync failed")

	if got := waitShutdown(deps, supervisor); got != 1 {
		t.Fatalf("waitShutdown returned %d, want 1", got)
	}
	if reason != "internal_failure" {
		t.Fatalf("session reason = %q", reason)
	}
	if failureComponent != "cache sync failed" ||
		failureCode != "api_unavailable" {
		t.Fatalf("failure evidence = %q/%q", failureComponent, failureCode)
	}
}

func TestRecordStartupFailureClassifiesKubernetesErrors(t *testing.T) {
	var component, code string
	deps := &serverDeps{
		recordFailure: func(_ context.Context, gotComponent, gotCode string) {
			component, code = gotComponent, gotCode
		},
	}
	recordStartupFailure(deps, errors.New("cache sync failed: API timeout"))
	if component != "cache sync failed" || code != "api_unavailable" {
		t.Fatalf("failure evidence = %q/%q", component, code)
	}
}

func TestWaitShutdownReturnsWhenApplicationContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	controllerDone := make(chan struct{})
	close(controllerDone)
	var reason string
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
		endSession: func(_ context.Context, value string) {
			reason = value
		},
	}
	supervisor := newComponentSupervisor(time.Now)
	cancel()

	if got := waitShutdown(deps, supervisor); got != 0 {
		t.Fatalf("waitShutdown returned %d, want 0", got)
	}
	if reason != "graceful_shutdown" {
		t.Fatalf("session reason = %q", reason)
	}
}
