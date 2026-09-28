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

func TestWaitControllerSkipsStandbyThatNeverStarted(t *testing.T) {
	deps := &serverDeps{controllerDone: make(chan struct{})}
	done := make(chan bool, 1)
	go func() { done <- waitController(deps) }()
	select {
	case stopped := <-done:
		if !stopped {
			t.Fatal("standby controller reported as not stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("standby shutdown waited for a controller that never ran")
	}
}

func TestWaitSaversReturnsTrueWhenAllDone(t *testing.T) {
	baselineDone := make(chan struct{})
	changeDone := make(chan struct{})
	incidentDone := make(chan struct{})
	feedbackDone := make(chan struct{})
	close(baselineDone)
	close(changeDone)
	close(incidentDone)
	close(feedbackDone)
	deps := &serverDeps{
		baselineDone: baselineDone,
		changeDone:   changeDone,
		incidentDone: incidentDone,
		feedbackDone: feedbackDone,
	}
	if !waitSavers(deps) {
		t.Fatal("waitSavers returned false when all done channels closed")
	}
}

func TestWaitSaversWaitsConcurrently(t *testing.T) {
	baselineDone := make(chan struct{})
	changeDone := make(chan struct{})
	incidentDone := make(chan struct{})
	feedbackDone := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(baselineDone)
		close(changeDone)
		close(incidentDone)
		close(feedbackDone)
		done <- struct{}{}
	}()
	deps := &serverDeps{
		baselineDone: baselineDone,
		changeDone:   changeDone,
		incidentDone: incidentDone,
		feedbackDone: feedbackDone,
	}
	result := make(chan bool, 1)
	go func() { result <- waitSavers(deps) }()
	select {
	case res := <-result:
		if !res {
			t.Fatal("waitSavers returned false")
		}
		select {
		case <-done:
		default:
			t.Fatal("channels were not closed before waitSavers returned")
		}
	case <-time.After(time.Second):
		t.Fatal("waitSavers did not return in time")
	}
}
