package incident

import (
	"sort"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// SnapshotPersisted returns all non-resolved incidents in serializable form.
func (e *Engine) SnapshotPersisted() []model.PersistedIncident {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotPersistedLocked()
}

// FreezeAndSnapshotPersisted stops runtime changes and returns final state.
// It is only for shutdown because freezing is permanent.
func (e *Engine) FreezeAndSnapshotPersisted() []model.PersistedIncident {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frozen = true
	return e.snapshotPersistedLocked()
}

// Caller must hold e.mu.
func (e *Engine) snapshotPersistedLocked() []model.PersistedIncident {
	keys := make([]string, 0, len(e.state)+len(e.massFailures))
	for key := range e.state {
		keys = append(keys, string(key))
	}
	for key := range e.massFailures {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	out := make([]model.PersistedIncident, 0, len(keys))
	for _, rawKey := range keys {
		key := model.IncidentKey(rawKey)
		inc := e.state[key]
		if inc == nil {
			inc = e.massFailures[key]
		}
		if inc == nil || inc.State == model.StateResolved {
			continue
		}
		out = append(out, inc.ToPersisted())
	}
	return out
}

// RestoreIncidents loads persisted incidents into the active state maps.
// Restored incidents are marked as already notified and refreshed so normal
// stale cleanup can close resources that no longer exist.
func (e *Engine) RestoreIncidents(
	incidents map[model.IncidentKey]*model.Incident,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	if len(incidents) == 0 {
		return
	}
	now := e.now()
	restored := 0
	for key, inc := range incidents {
		if inc == nil {
			continue
		}
		if IsMassFailureKey(key) {
			if _, exists := e.massFailures[key]; exists {
				continue
			}
			e.restoreMassFailure(key, inc, now)
			restored++
			continue
		}
		restoreKey := key
		if _, ok := e.baseline[string(restoreKey)]; !ok {
			if migrated, ok := e.migrateLegacyPodKey(key, inc); ok {
				restoreKey = migrated
			}
		}
		if _, exists := e.state[restoreKey]; exists {
			continue
		}
		clone := inc.Clone()
		clone.Key = restoreKey
		prepareRestoredIncident(clone, now)
		e.state[restoreKey] = clone
		e.indexIncident(clone)
		restored++
	}
	if restored > 0 {
		klog.InfoS("restored incidents from ConfigMap", "count", restored)
	}
}

func (e *Engine) restoreMassFailure(
	key model.IncidentKey,
	inc *model.Incident,
	now time.Time,
) {
	if _, exists := e.massFailures[key]; exists {
		return
	}
	clone := inc.Clone()
	if clone.Fingerprint == "" {
		clone.Fingerprint = legacyFingerprint(clone.Key)
	}
	clone.LastSeen = now
	clone.LastUpdate = now
	clone.NotifiedSig = notifSig(clone)
	e.massFailures[key] = clone
}

func prepareRestoredIncident(inc *model.Incident, now time.Time) {
	if inc.Fingerprint == "" {
		inc.Fingerprint = legacyFingerprint(inc.Key)
	}
	inc.LastSeen = now
	inc.LastUpdate = now
	inc.NotifiedSig = notifSig(inc)
}
