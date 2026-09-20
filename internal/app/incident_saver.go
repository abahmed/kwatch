package app

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// stateSnapshot is the incident state sent to persistence writers. The parts
// are stored independently so partial auxiliary writes remain recoverable.
type stateSnapshot struct {
	incidents []model.PersistedIncident
	groups    []model.PersistedGroup
	// threads is provider name → incident key → conversation ID.
	threads map[string]map[string]string
	// engine contains cooldowns, pod UIDs, container states, and fan-out data.
	engine model.PersistedEngineState
}

func trySendIncidentSnapshot(ch chan stateSnapshot, snap stateSnapshot) {
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

// incidentSaver is the typed contract used by the incident state writer.
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
	persistenceManager incidentSaver,
	ch <-chan stateSnapshot,
	report func(error),
	canWrite func() bool,
	progress func(),
) error {
	var pending stateSnapshot
	var havePending bool
	// Avoid repeated writes when lifecycle ticks serialize identical state.
	var lastSaved uint64
	stopHeartbeat := startProgressHeartbeat(ctx, progress)
	defer stopHeartbeat()
	for {
		if !writesAllowed(canWrite) {
			return errComponentCleanStop
		}
		select {
		case snap := <-ch:
			if progress != nil {
				progress()
			}
			pending, havePending = snap, true
			var err error
			lastSaved, err = saveIncidentSnapshot(
				ctx, persistenceManager, pending, 10*time.Second, lastSaved,
				report, canWrite,
			)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			for {
				select {
				case snap := <-ch:
					pending, havePending = snap, true
				default:
					if havePending {
						fctx, cancel := finalWriteContext(ctx)
						_, _ = saveIncidentSnapshot(
							fctx, persistenceManager, pending,
							5*time.Second, lastSaved,
							report, canWrite,
						)
						cancel()
					}
					return nil
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
	case <-time.After(componentShutdownTimeout):
		recordShutdownTimeout("incident-saver")
		return false
	}
}

func waitPersistenceComponent(done <-chan struct{}, name string) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(componentShutdownTimeout):
		recordShutdownTimeout(name)
		return false
	}
}

func saveFinalIncidentSnapshot(ctx context.Context, deps *serverDeps) {
	if deps.incidentSaver == nil || deps.incidentEngine == nil {
		return
	}
	// Freeze incident state before saving groups so both snapshots agree.
	groups := deps.incidentEngine.SnapshotGroups()
	var threads map[string]map[string]string
	if deps.deliveryManager != nil {
		threads = deps.deliveryManager.SnapshotThreads()
	}
	engineState := deps.incidentEngine.SnapshotEngineState()
	_, _ = saveIncidentSnapshot(
		ctx, deps.incidentSaver,
		stateSnapshot{
			incidents: deps.incidentEngine.FreezeAndSnapshotPersisted(),
			groups:    groups,
			threads:   threads,
			engine:    engineState,
		},
		5*time.Second,
		// Do not write after the process has lost the Lease.
		0,
		nil,
		deps.persistenceGate.enabled,
	)
}

func saveIncidentSnapshot(
	ctx context.Context,
	persistenceManager incidentSaver,
	snap stateSnapshot,
	timeout time.Duration,
	lastSaved uint64,
	report func(error),
	canWrite func() bool,
) (uint64, error) {
	if !writesAllowed(canWrite) {
		return lastSaved, nil
	}
	sig, ok := snapshotFingerprint(snap)
	if ok && sig == lastSaved {
		return lastSaved, nil
	}
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := persistenceManager.SaveIncidentState(
		fctx, snap.incidents, snap.groups, snap.threads, snap.engine,
	)
	if err != nil {
		klog.ErrorS(err, "failed to save correlation state")
		if report != nil {
			report(err)
		}
		return lastSaved, err
	}
	if report != nil {
		report(nil)
	}
	if !ok {
		return lastSaved, nil
	}
	return sig, nil
}

// snapshotFingerprint provides a deterministic write-suppression key.
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
