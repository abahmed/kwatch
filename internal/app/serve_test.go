package app

import (
	"context"
	"errors"
	"testing"

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
		cancel:          cancel,
		controllerDone:  controllerDone,
		deliveryManager: delivery.NewManager(),
		healthServer:    health.NewHealthServer(config.HealthCheck{}),
		cleanup:         func() {},
	}
	supervisor := newComponentSupervisor()
	supervisor.errCh <- errors.New("cache sync failed")

	if got := waitShutdown(deps, supervisor); got != 1 {
		t.Fatalf("waitShutdown returned %d, want 1", got)
	}
}
