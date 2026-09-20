package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

const (
	optionalRestartInitial  = time.Second
	optionalRestartMaximum  = time.Minute
	defaultComponentStartup = 2 * leaderRenewDeadline
	defaultComponentStall   = 2 * leaderRenewDeadline
)

var errComponentStopped = errors.New("component stopped unexpectedly")

var errComponentCleanStop = errors.New("component stopped cleanly")

var errComponentStalled = errors.New("component stalled")

var errComponentShutdown = errors.New("component shutdown timed out")

// componentSupervisor owns application-launched goroutines and the single
// failure channel observed by the shutdown loop. Domain components keep their
// own run and stop semantics; the application owns their lifetime.
type componentSupervisor struct {
	wg    sync.WaitGroup
	errCh chan error
	now   func() time.Time
}

func newComponentSupervisor(now func() time.Time) *componentSupervisor {
	if now == nil {
		panic("component supervisor clock is required")
	}
	return &componentSupervisor{errCh: make(chan error, 1), now: now}
}

func (s *componentSupervisor) startOwned(
	ctx context.Context,
	component componentSpec,
) {
	if component.run == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if component.onHealthy != nil {
			component.onHealthy()
		}
		err := s.runComponent(ctx, component)
		if errors.Is(err, errComponentCleanStop) {
			return
		}
		if err == nil && ctx.Err() == nil {
			err = errComponentStopped
			metrics.DefaultRegistry().ComponentUnexpectedStops.Add(1)
		}
		if err != nil {
			if component.required {
				s.report(fmt.Errorf("%s: %w", component.name, err))
				return
			}
			if errors.Is(err, errComponentShutdown) {
				if component.onError != nil {
					component.onError(err)
				}
				return
			}
			if component.onError != nil {
				component.onError(err)
			}
			klog.ErrorS(err, "application component stopped",
				"component", component.name)
		}
	}()
}

func (s *componentSupervisor) startOptional(
	ctx context.Context,
	initialized <-chan struct{},
	component componentSpec,
) {
	if component.run == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if !waitForInitialization(ctx, initialized) {
			return
		}
		delay := optionalRestartInitial
		klog.InfoS(
			"starting optional component",
			"component", component.name,
			"required", component.required,
		)
		for {
			startedAt := s.now()
			if component.onHealthy != nil {
				component.onHealthy()
			}
			err := s.runComponent(ctx, component)
			if errors.Is(err, errComponentCleanStop) {
				return
			}
			if err == nil && ctx.Err() == nil {
				err = errComponentStopped
				metrics.DefaultRegistry().ComponentUnexpectedStops.Add(1)
			}
			if ctx.Err() != nil &&
				(err == nil || errors.Is(err, context.Canceled)) {
				return
			}
			if errors.Is(err, errComponentShutdown) {
				if component.onError != nil {
					component.onError(err)
				}
				// A component that ignores cancellation cannot be safely
				// restarted in this process. Escalate the timeout so the
				// application exits and Kubernetes can replace the Pod.
				s.report(fmt.Errorf("%s: %w", component.name, err))
				return
			}
			if component.required {
				s.report(fmt.Errorf("%s: %w", component.name, err))
				return
			}
			if component.onError != nil {
				component.onError(err)
			}
			klog.ErrorS(err, "optional component stopped; retrying",
				"component", component.name, "retryIn", delay)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
			if delay < optionalRestartMaximum/2 {
				delay *= 2
			} else {
				delay = optionalRestartMaximum
			}
			if s.now().Sub(startedAt) >= time.Minute {
				delay = optionalRestartInitial
			}
		}
	}()
}

func (s *componentSupervisor) runComponent(
	ctx context.Context,
	component componentSpec,
) error {
	if component.run == nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- component.run(runCtx) }()

	stallTimeout := component.stallTimeout
	if stallTimeout == 0 {
		stallTimeout = defaultComponentStall
	}
	startupTimeout := component.startupTimeout
	if startupTimeout == 0 {
		startupTimeout = defaultComponentStartup
	}
	if component.progress == nil || stallTimeout <= 0 {
		return waitForComponent(ctx, runCtx, cancel, runDone)
	}

	startedAt := s.now()
	ticker := time.NewTicker(progressCheckInterval(stallTimeout))
	defer ticker.Stop()
	for {
		select {
		case err := <-runDone:
			if err == nil && ctx.Err() == nil {
				return errComponentStopped
			}
			return err
		case <-ctx.Done():
			cancel()
			return waitForComponent(ctx, runCtx, cancel, runDone)
		case <-ticker.C:
			last := component.progress.LastProgress()
			if last.IsZero() {
				last = startedAt
				if s.now().Sub(last) > startupTimeout {
					cancel()
					metrics.DefaultRegistry().ComponentStalls.Add(1)
					return waitAfterCancellation(errComponentStalled, runDone)
				}
				continue
			}
			if s.now().Sub(last) > stallTimeout {
				cancel()
				metrics.DefaultRegistry().ComponentStalls.Add(1)
				return waitAfterCancellation(errComponentStalled, runDone)
			}
		}
	}
}

func waitForComponent(
	parent context.Context,
	runCtx context.Context,
	cancel context.CancelFunc,
	done <-chan error,
) error {
	select {
	case err := <-done:
		return err
	case <-parent.Done():
		cancel()
		return waitAfterCancellation(parent.Err(), done)
	case <-runCtx.Done():
		return waitAfterCancellation(runCtx.Err(), done)
	}
}

// waitAfterCancellation prevents an optional component from being restarted
// while its previous generation is still executing. A component that ignores
// cancellation is reported as a bounded shutdown failure and is never
// relaunched by the supervisor.
func waitAfterCancellation(
	result error,
	done <-chan error,
) error {
	timer := time.NewTimer(componentShutdownTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return result
	case <-timer.C:
		return fmt.Errorf("%w: %v", errComponentShutdown, result)
	}
}

func progressCheckInterval(timeout time.Duration) time.Duration {
	interval := timeout / 4
	if interval < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return interval
}

func (s *componentSupervisor) report(err error) {
	if err == nil {
		return
	}
	select {
	case s.errCh <- err:
	default:
		klog.ErrorS(err, "application component failure was already reported")
	}
}
