package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
)

func TestWaitShutdownReturnsFailureForControllerError(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	controllerDone := make(chan struct{})
	close(controllerDone)
	errCh := make(chan error, 1)
	errCh <- errors.New("cache sync failed")

	deps := &serverDeps{
		cancel:         cancel,
		controllerDone: controllerDone,
		alertManager:   &alert.AlertManager{},
		healthServer:   health.NewHealthServer(config.HealthCheck{}),
		cleanup:        func() {},
	}
	var wg sync.WaitGroup

	if got := waitShutdown(deps, &wg, errCh); got != 1 {
		t.Fatalf("waitShutdown returned %d, want 1", got)
	}
}
