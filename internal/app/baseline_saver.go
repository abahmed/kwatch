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
	if interval <= 0 {
		interval = 10 * time.Second
	}
	var pending map[string]map[string]int64
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case b := <-ch:
			pending = b
			if timer == nil {
				timer = time.NewTimer(interval)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(interval)
			}
			timerC = timer.C
		case <-timerC:
			if err := persistenceManager.SaveBaseline(
				context.Background(), pending,
			); err != nil {
				klog.ErrorS(err, "failed to save baseline")
			}
			timerC = nil
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			if pending != nil {
				fctx, cancel := context.WithTimeout(
					context.Background(),
					5*time.Second,
				)
				if err := persistenceManager.SaveBaseline(fctx, pending); err != nil {
					klog.ErrorS(err, "failed to save final baseline")
				}
				cancel()
			}
			return
		}
	}
}
