package app

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

type feedbackSaver interface {
	SaveRCAFeedback(context.Context, []insight.RCARecord) error
}

func trySendFeedbackSnapshot(ch chan []insight.RCARecord, snapshot []insight.RCARecord) {
	select {
	case ch <- snapshot:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snapshot:
		default:
		}
	}
}

// startFeedbackSaver keeps ConfigMap I/O out of the incident lifecycle hook.
// Feedback is advisory state, so the newest coalesced snapshot is sufficient.
func startFeedbackSaver(
	ctx context.Context,
	stateMgr feedbackSaver,
	ch <-chan []insight.RCARecord,
	done chan<- struct{},
) {
	defer close(done)
	var pending []insight.RCARecord
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	save := func(timeout time.Duration) {
		if pending == nil {
			return
		}
		fctx, cancel := context.WithTimeout(context.Background(), timeout)
		err := stateMgr.SaveRCAFeedback(fctx, pending)
		if err != nil {
			klog.ErrorS(err, "failed to persist RCA feedback")
			cancel()
			return
		}
		cancel()
		pending = nil
	}
	for {
		select {
		case snapshot := <-ch:
			pending = snapshot
		case <-ticker.C:
			save(5 * time.Second)
		case <-ctx.Done():
			for {
				select {
				case snapshot := <-ch:
					pending = snapshot
				default:
					save(5 * time.Second)
					return
				}
			}
		}
	}
}

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
// stateSnapshot is the correlation snapshot sent to the state writers. The
// payloads are stored in dedicated ConfigMaps, so a restart can observe a
// partially newer auxiliary snapshot and safely discard references to absent
// incidents.
type stateSnapshot struct {
	incidents []model.PersistedIncident
	groups    []model.PersistedGroup
	// threads is provider name → incident key → conversation id, so a resolve
	// posted after a restart still lands under the alert that opened it.
	threads map[string]map[string]string
	// engine is the correlation bookkeeping that is neither an incident nor a
	// group: cooldowns, pod UIDs, container states, fan-out windows.
	engine model.PersistedEngineState
}

func trySendIncidentSnapshot(
	ch chan stateSnapshot,
	snap stateSnapshot,
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
	SaveIncidentState(
		ctx context.Context,
		incidents []model.PersistedIncident,
		groups []model.PersistedGroup,
		threads map[string]map[string]string,
		engine model.PersistedEngineState,
	) error
}

func startIncidentSaver(
	ctx context.Context,
	stateMgr incidentSaver,
	ch <-chan stateSnapshot,
) {
	var pending stateSnapshot
	var havePending bool
	// lastSaved is the fingerprint of what is already in the ConfigMap. A
	// snapshot identical to it is not written: on a cluster with a few
	// long-running incidents the engine reports a change every lifecycle
	// tick whose serialized form is byte-for-byte the same, and each one
	// used to be a ConfigMap write.
	var lastSaved uint64
	for {
		select {
		case snap := <-ch:
			pending, havePending = snap, true
			lastSaved = saveIncidentSnapshot(
				stateMgr, pending, 10*time.Second, lastSaved,
			)
		case <-ctx.Done():
			for {
				select {
				case snap := <-ch:
					pending, havePending = snap, true
				default:
					if havePending {
						saveIncidentSnapshot(
							stateMgr, pending, 5*time.Second, lastSaved,
						)
					}
					return
				}
			}
		}
	}
}

func waitIncidentSaver(deps *serverDeps) bool {
	if deps.incidentDone == nil {
		return true
	}
	select {
	case <-deps.incidentDone:
		return true
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for incident saver")
		return false
	}
}

func saveFinalIncidentSnapshot(deps *serverDeps) {
	if deps.incidentSaver == nil || deps.correlator == nil {
		return
	}
	// Groups first: freezing is permanent, and the group snapshot reads the
	// same state the incident snapshot is about to freeze.
	groups := deps.correlator.SnapshotGroups()
	var threads map[string]map[string]string
	if deps.alertManager != nil {
		threads = deps.alertManager.SnapshotThreads()
	}
	engineState := deps.correlator.SnapshotEngineState()
	saveIncidentSnapshot(
		deps.incidentSaver,
		stateSnapshot{
			incidents: deps.correlator.FreezeAndSnapshotPersisted(),
			groups:    groups,
			threads:   threads,
			engine:    engineState,
		},
		5*time.Second,
		// The final write is unconditional: this is the shutdown snapshot and
		// there is no next chance to correct it.
		0,
	)
}

func waitFeedbackSaver(deps *serverDeps) bool {
	if deps.feedbackDone == nil {
		return true
	}
	select {
	case <-deps.feedbackDone:
		return true
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for RCA feedback saver")
		return false
	}
}

// saveIncidentSnapshot writes the snapshot unless it matches lastSaved, and
// returns the fingerprint now on record.
//
// The four parts go to dedicated ConfigMaps. The state writer orders auxiliary
// writes before cleaning legacy keys to keep upgrades recoverable.
func saveIncidentSnapshot(
	stateMgr incidentSaver,
	snap stateSnapshot,
	timeout time.Duration,
	lastSaved uint64,
) uint64 {
	sig, ok := snapshotFingerprint(snap)
	if ok && sig == lastSaved {
		return lastSaved
	}
	fctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := stateMgr.SaveIncidentState(
		fctx, snap.incidents, snap.groups, snap.threads, snap.engine,
	)
	if err != nil {
		klog.ErrorS(err, "failed to save correlation state")
		return lastSaved
	}
	if !ok {
		return lastSaved
	}
	return sig
}

// snapshotFingerprint hashes the serialized snapshot. ok is false when it
// cannot be serialized, in which case the caller writes unconditionally
// rather than skipping on a fingerprint it does not have.
func snapshotFingerprint(snap stateSnapshot) (uint64, bool) {
	h := fnv.New64a()
	enc := json.NewEncoder(h)
	for _, part := range []any{
		snap.incidents, snap.groups, snap.threads, snap.engine,
	} {
		if err := enc.Encode(part); err != nil {
			return 0, false
		}
	}
	return h.Sum64(), true
}

func renotifyIntervalBySeverity(m map[string]int) map[string]time.Duration {
	r := make(map[string]time.Duration, len(m))
	for k, v := range m {
		r[k] = time.Duration(v) * time.Minute
	}
	return r
}
