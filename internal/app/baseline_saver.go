package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"
)

// startBaselineSaver coalesces baseline writes: the latest snapshot wins.
func startBaselineSaver(
	ctx context.Context,
	persistenceManager interface {
		SaveBaseline(context.Context, map[string]map[string]int64) error
	},
	ch <-chan map[string]map[string]int64,
	interval time.Duration,
) {
	_ = startBaselineSaverWithStatus(
		ctx, persistenceManager, ch, interval, nil, nil, nil,
	)
}

func startBaselineSaverWithStatus(
	ctx context.Context,
	persistenceManager interface {
		SaveBaseline(context.Context, map[string]map[string]int64) error
	},
	ch <-chan map[string]map[string]int64,
	interval time.Duration,
	report func(error),
	canWrite func() bool,
	progress func(),
) error {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	state := baselineSaverState{}
	stopHeartbeat := startProgressHeartbeat(ctx, progress)
	defer stopHeartbeat()
	for {
		select {
		case b := <-ch:
			state.accept(b, interval, progress)
		case <-state.timerC:
			if progress != nil {
				progress()
			}
			if !writesAllowed(canWrite) {
				return errComponentCleanStop
			}
			state.save(ctx, persistenceManager, canWrite, report)
		case <-state.retryC:
			state.retry(ctx, persistenceManager, canWrite, report)
		case <-ctx.Done():
			state.finish(ctx, persistenceManager, canWrite, report)
			return nil
		}
	}
}

type baselineSaverState struct {
	pending    map[string]map[string]int64
	timer      *time.Timer
	timerC     <-chan time.Time
	retryTimer *time.Timer
	retryC     <-chan time.Time
	retryDelay time.Duration
}

func (s *baselineSaverState) accept(
	baseline map[string]map[string]int64,
	interval time.Duration,
	progress func(),
) {
	if baseline == nil {
		return
	}
	if progress != nil {
		progress()
	}
	s.pending = baseline
	s.timer = resetBaselineTimer(s.timer, interval)
	s.timerC = s.timer.C
}

func (s *baselineSaverState) save(
	ctx context.Context,
	persistenceManager interface {
		SaveBaseline(context.Context, map[string]map[string]int64) error
	},
	canWrite func() bool,
	report func(error),
) {
	err := saveBaseline(ctx, persistenceManager, s.pending, canWrite)
	if err != nil {
		klog.ErrorS(err, "failed to save baseline")
		if report != nil {
			report(err)
		}
		s.scheduleRetry()
		return
	}
	s.resetRetry(report)
}

func (s *baselineSaverState) retry(
	ctx context.Context,
	persistenceManager interface {
		SaveBaseline(context.Context, map[string]map[string]int64) error
	},
	canWrite func() bool,
	report func(error),
) {
	if s.pending == nil || !writesAllowed(canWrite) {
		return
	}
	err := saveBaseline(ctx, persistenceManager, s.pending, canWrite)
	if err != nil {
		klog.ErrorS(err, "failed to retry baseline")
		if report != nil {
			report(err)
		}
		s.scheduleRetry()
		return
	}
	s.resetRetry(report)
}

func (s *baselineSaverState) scheduleRetry() {
	if s.retryDelay == 0 {
		s.retryDelay = time.Second
	}
	s.retryTimer = resetRetryTimer(s.retryTimer, s.retryDelay)
	s.retryC = s.retryTimer.C
	s.retryDelay *= 2
	if s.retryDelay > time.Minute {
		s.retryDelay = time.Minute
	}
}

func (s *baselineSaverState) resetRetry(report func(error)) {
	if report != nil {
		report(nil)
	}
	s.retryDelay = time.Second
	stopRetryTimer(s.retryTimer)
	s.retryC = nil
}

func (s *baselineSaverState) finish(
	ctx context.Context,
	persistenceManager interface {
		SaveBaseline(context.Context, map[string]map[string]int64) error
	},
	canWrite func() bool,
	report func(error),
) {
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.pending == nil || !writesAllowed(canWrite) {
		return
	}
	fctx, cancel := finalWriteContext(ctx)
	err := persistenceManager.SaveBaseline(fctx, s.pending)
	if err != nil {
		klog.ErrorS(err, "failed to save final baseline")
		if report != nil {
			report(err)
		}
	} else if report != nil {
		report(nil)
	}
	cancel()
}

func resetBaselineTimer(timer *time.Timer, interval time.Duration) *time.Timer {
	if timer == nil {
		return time.NewTimer(interval)
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(interval)
	return timer
}

func saveBaseline(
	ctx context.Context,
	persistenceManager interface {
		SaveBaseline(context.Context, map[string]map[string]int64) error
	},
	pending map[string]map[string]int64,
	canWrite func() bool,
) error {
	if !writesAllowed(canWrite) {
		return context.Canceled
	}
	fctx, cancel := context.WithTimeout(
		ctx, 5*time.Second,
	)
	defer cancel()
	return persistenceManager.SaveBaseline(fctx, pending)
}
