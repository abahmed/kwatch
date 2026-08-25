package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// startBaselineSaver coalesces baseline writes: at most one ConfigMap write
// every interval. The latest snapshot always wins. Use 0 for the default
// interval (10 seconds).
func startBaselineSaver(ctx context.Context, stateMgr interface {
	SaveBaseline(context.Context, map[string]map[string]int64) error
}, ch <-chan map[string]map[string]int64, interval time.Duration) {
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
			if err := stateMgr.SaveBaseline(context.Background(), pending); err != nil {
				klog.ErrorS(err, "failed to save baseline")
			}
			timerC = nil
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			if pending != nil {
				fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = stateMgr.SaveBaseline(fctx, pending)
				cancel()
			}
			return
		}
	}
}

// startIncidentSaver saves incident snapshots to the ConfigMap whenever a
// snapshot arrives on the channel. On ctx cancellation it saves the final
// snapshot before returning.
func trySendIncidentSnapshot(ch chan<- []model.PersistedIncident, snap []model.PersistedIncident) {
	select {
	case ch <- snap:
	default:
		klog.V(4).InfoS("incident snapshot channel full, dropping")
	}
}

func startIncidentSaver(ctx context.Context, stateMgr interface {
	SaveIncidents(context.Context, any) error
}, ch <-chan []model.PersistedIncident) {
	var pending []model.PersistedIncident
	for {
		select {
		case snap := <-ch:
			pending = snap
			fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := stateMgr.SaveIncidents(fctx, pending); err != nil {
				klog.ErrorS(err, "failed to save incidents")
			}
			cancel()
		case <-ctx.Done():
			if len(pending) > 0 {
				fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = stateMgr.SaveIncidents(fctx, pending)
				cancel()
			}
			return
		}
	}
}

func renotifyIntervalBySeverity(m map[string]int) map[string]time.Duration {
	r := make(map[string]time.Duration, len(m))
	for k, v := range m {
		r[k] = time.Duration(v) * time.Minute
	}
	return r
}
