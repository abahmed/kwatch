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
			if err := stateMgr.SaveBaseline(
				context.Background(),
				pending,
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
				if err := stateMgr.SaveBaseline(fctx, pending); err != nil {
					klog.ErrorS(err, "failed to save final baseline")
				}
				cancel()
			}
			return
		}
	}
}

// startIncidentSaver saves incident snapshots to the ConfigMap whenever a
// snapshot arrives on the channel. On ctx cancellation it saves the final
// snapshot before returning.
func trySendIncidentSnapshot(
	ch chan []model.PersistedIncident,
	snap []model.PersistedIncident,
) {
	select {
	case ch <- snap:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snap:
		default:
			klog.V(4).InfoS("incident snapshot channel full, dropping")
		}
	}
}

// incidentSaver is the narrow contract the saver needs. It is typed on
// purpose: an `any` here is what previously let the saved shape and the
// restored shape drift apart unnoticed.
type incidentSaver interface {
	SavePersistedIncidents(context.Context, []model.PersistedIncident) error
}

func startIncidentSaver(
	ctx context.Context,
	stateMgr incidentSaver,
	ch <-chan []model.PersistedIncident,
) {
	var pending []model.PersistedIncident
	for {
		select {
		case snap := <-ch:
			pending = snap
			saveIncidentSnapshot(stateMgr, pending, 10*time.Second)
		case <-ctx.Done():
			for {
				select {
				case snap := <-ch:
					pending = snap
				default:
					if pending != nil {
						saveIncidentSnapshot(stateMgr, pending, 5*time.Second)
					}
					return
				}
			}
		}
	}
}

func waitIncidentSaver(deps *serverDeps) {
	if deps.incidentDone == nil {
		return
	}
	select {
	case <-deps.incidentDone:
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for incident saver")
	}
}

func saveFinalIncidentSnapshot(deps *serverDeps) {
	if deps.incidentSaver == nil || deps.correlator == nil {
		return
	}
	saveIncidentSnapshot(
		deps.incidentSaver,
		deps.correlator.SnapshotPersisted(),
		5*time.Second,
	)
}

func saveIncidentSnapshot(
	stateMgr incidentSaver,
	snap []model.PersistedIncident,
	timeout time.Duration,
) {
	fctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := stateMgr.SavePersistedIncidents(fctx, snap); err != nil {
		klog.ErrorS(err, "failed to save incidents")
	}
}

func renotifyIntervalBySeverity(m map[string]int) map[string]time.Duration {
	r := make(map[string]time.Duration, len(m))
	for k, v := range m {
		r[k] = time.Duration(v) * time.Minute
	}
	return r
}
