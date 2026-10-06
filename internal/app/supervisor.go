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
		err, stop := normalizeComponentExit(
			ctx, component, s.runComponent(ctx, component))
		if !stop {
			s.handleOwnedExit(component, err)
		}
	}()
}

// handleOwnedExit reacts to a run that ended with err. A required
// component's failure ends the application; an optional one is logged.
func (s *componentSupervisor) handleOwnedExit(
	component componentSpec, err error,
) {
	if err == nil {
		return
	}
	if component.required {
		s.report(fmt.Errorf("%s: %w", component.name, err))
		return
	}
	if component.onError != nil {
		component.onError(err)
	}
	if errors.Is(err, errComponentShutdown) {
		return
	}
	klog.ErrorS(err, "application component stopped",
		"component", component.name)
}

func (s *componentSupervisor) startOptional(
	ctx context.Context,
	component componentSpec,
) {
	if component.run == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.restartLoop(ctx, component)
	}()
}

// restartLoop runs an optional component again, with backoff, until ctx
// ends or the component fails in a way that must not be retried.
func (s *componentSupervisor) restartLoop(
	ctx context.Context, component componentSpec,
) {
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
		var sleep time.Duration
		sleep, delay = optionalBackoff(delay, s.now().Sub(startedAt))
		err, stop := normalizeComponentExit(ctx, component, err)
		if stop || s.handleTerminalOptionalExit(component, err) {
			return
		}
		if component.onError != nil {
			component.onError(err)
		}
		klog.ErrorS(err, "optional component stopped; retrying",
			"component", component.name, "retryIn", sleep)
		if !sleepOrDone(ctx, sleep) {
			return
		}
	}
}

// sleepOrDone waits for d and reports false when ctx ended first.
func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return false
	case <-timer.C:
		return true
	}
}

func normalizeComponentExit(
	ctx context.Context,
	component componentSpec,
	err error,
) (error, bool) {
	if errors.Is(err, errComponentCleanStop) {
		return nil, true
	}
	if err == nil && ctx.Err() == nil {
		if component.cleanStop {
			return nil, true
		}
		err = errComponentStopped
		metrics.DefaultRegistry().ComponentUnexpectedStops.Add(1)
	}
	if ctx.Err() != nil &&
		(err == nil || errors.Is(err, context.Canceled)) {
		return nil, true
	}
	return err, false
}

func (s *componentSupervisor) handleTerminalOptionalExit(
	component componentSpec,
	err error,
) bool {
	if errors.Is(err, errComponentShutdown) {
		if component.onError != nil {
			component.onError(err)
		}
		// A component that ignores cancellation cannot be safely restarted.
		s.report(fmt.Errorf("%s: %w", component.name, err))
		return true
	}
	if component.required {
		s.report(fmt.Errorf("%s: %w", component.name, err))
		return true
	}
	return false
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

	stallTimeout := durationOr(component.stallTimeout, defaultComponentStall)
	startupTimeout := durationOr(
		component.startupTimeout, defaultComponentStartup)
	if component.progress == nil || stallTimeout <= 0 {
		err := waitForComponent(ctx, runCtx, cancel, runDone)
		if err == nil && component.cleanStop {
			return errComponentCleanStop
		}
		return err
	}
	return s.watchProgress(runWatch{
		parent: ctx, runCtx: runCtx, cancel: cancel, done: runDone,
		component: component, stall: stallTimeout, startup: startupTimeout,
	})
}

func durationOr(d, fallback time.Duration) time.Duration {
	if d == 0 {
		return fallback
	}
	return d
}

// runWatch is everything watchProgress needs to supervise one run.
type runWatch struct {
	parent    context.Context
	runCtx    context.Context
	cancel    context.CancelFunc
	done      <-chan error
	component componentSpec
	// stall bounds the time without progress once progress was reported;
	// startup bounds the wait for the first report.
	stall   time.Duration
	startup time.Duration
}

// watchProgress waits for the run to end and cancels it when it stops
// reporting progress.
func (s *componentSupervisor) watchProgress(w runWatch) error {
	startedAt := s.now()
	ticker := time.NewTicker(progressCheckInterval(w.stall))
	defer ticker.Stop()
	for {
		select {
		case err := <-w.done:
			return unexpectedReturn(w.parent, w.component, err)
		case <-w.parent.Done():
			w.cancel()
			return waitForComponent(w.parent, w.runCtx, w.cancel, w.done)
		case <-ticker.C:
			last := w.component.progress.LastProgress()
			if s.progressStalled(last, startedAt, w) {
				w.cancel()
				metrics.DefaultRegistry().ComponentStalls.Add(1)
				return waitAfterCancellation(errComponentStalled, w.done)
			}
		}
	}
}

// progressStalled reports whether the component went too long without
// progress. Before its first report the startup deadline applies.
func (s *componentSupervisor) progressStalled(
	last, startedAt time.Time, w runWatch,
) bool {
	if last.IsZero() {
		return s.now().Sub(startedAt) > w.startup
	}
	return s.now().Sub(last) > w.stall
}

// unexpectedReturn turns a nil return before cancellation into an error,
// because a running component is not supposed to finish by itself.
func unexpectedReturn(
	parent context.Context, component componentSpec, err error,
) error {
	if err == nil && parent.Err() == nil {
		if component.cleanStop {
			return errComponentCleanStop
		}
		return errComponentStopped
	}
	return err
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
		// result is formatted with %v on purpose: wrapping a context
		// error would let the supervisor read the timeout as a clean
		// cancellation and hide the shutdown failure.
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

// optionalBackoff returns how long to wait before restarting an optional
// component and the delay to use after that. A run that stayed up for a
// minute was healthy, so it restarts promptly; the run time is measured
// before sleeping so the backoff itself never counts as healthy runtime.
func optionalBackoff(
	delay, ranFor time.Duration,
) (time.Duration, time.Duration) {
	if ranFor >= time.Minute || delay <= 0 {
		delay = optionalRestartInitial
	}
	next := delay * 2
	if next > optionalRestartMaximum {
		next = optionalRestartMaximum
	}
	return delay, next
}
