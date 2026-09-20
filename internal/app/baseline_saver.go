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
	var pending map[string]map[string]int64
	var timer *time.Timer
	var timerC <-chan time.Time
	stopHeartbeat := startProgressHeartbeat(ctx, progress)
	defer stopHeartbeat()
	for {
		select {
		case b := <-ch:
			if b == nil {
				continue
			}
			if progress != nil {
				progress()
			}
			pending = b
			timer = resetBaselineTimer(timer, interval)
			timerC = timer.C
		case <-timerC:
			if progress != nil {
				progress()
			}
			if !writesAllowed(canWrite) {
				return errComponentCleanStop
			}
			if err := saveBaseline(
				ctx, persistenceManager, pending, canWrite,
			); err != nil {
				klog.ErrorS(err, "failed to save baseline")
				if report != nil {
					report(err)
				}
				return err
			} else if report != nil {
				report(nil)
			}
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			if pending != nil && writesAllowed(canWrite) {
				fctx, cancel := finalWriteContext(ctx)
				if err := persistenceManager.SaveBaseline(fctx, pending); err != nil {
					klog.ErrorS(err, "failed to save final baseline")
					if report != nil {
						report(err)
					}
				} else if report != nil {
					report(nil)
				}
				cancel()
			}
			return nil
		}
	}
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
