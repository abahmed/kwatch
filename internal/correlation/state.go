package correlation

import (
	"sort"

	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) ActiveCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, inc := range e.state {
		if inc.State != model.StateResolved {
			n++
		}
	}
	return n
}

func (e *Engine) Snapshot() []model.IncidentView {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.IncidentView, 0, len(e.state))
	for _, inc := range e.state {
		out = append(out, model.IncidentView{
			Key:       inc.Key,
			Reason:    inc.Reason,
			Namespace: inc.Namespace,
			Name:      inc.Name,
			State:     inc.State,
			Severity:  inc.Severity,
			Count:     inc.Count,
			FirstSeen: inc.FirstSeen,
			LastSeen:  inc.LastSeen,
			Hint:      inc.Hint,
		})
	}
	return out
}

func (e *Engine) GetLastContainerState(
	namespace, podName, container string,
) *model.ContainerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := lastContainerKey(namespace, podName, container)
	cs, ok := e.lastContainerIndex[key]
	if !ok || cs == nil {
		return nil
	}
	cp := *cs
	return &cp
}

func lastContainerKey(namespace, podName, container string) string {
	if container == "" || container == "." {
		container = "."
	}
	return namespace + "/" + podName + "/" + container
}

// Caller must hold e.mu.
func (e *Engine) indexLastContainerState(
	namespace, podName, container string,
	cs *model.ContainerState,
) {
	if podName == "" || cs == nil {
		return
	}
	cp := *cs
	e.lastContainerIndex[lastContainerKey(namespace, podName, container)] = &cp
}

// Caller must hold e.mu.
func (e *Engine) indexIncidentByNamespace(inc *model.Incident) {
	ns, key := inc.Namespace, inc.Key
	if ns == "" {
		return
	}
	if e.namespaceIndex[ns] == nil {
		e.namespaceIndex[ns] = make(map[model.IncidentKey]*model.Incident)
	}
	e.namespaceIndex[ns][key] = inc
}

// Caller must hold e.mu.
func (e *Engine) removeIncidentFromNamespaceIndex(inc *model.Incident) {
	ns, key := inc.Namespace, inc.Key
	if ns == "" {
		return
	}
	delete(e.namespaceIndex[ns], key)
	if len(e.namespaceIndex[ns]) == 0 {
		delete(e.namespaceIndex, ns)
	}
}

func (e *Engine) SetDeployLister(l appsv1lister.DeploymentLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deployLister = l
}

func (e *Engine) SetStatefulSetLister(l appsv1lister.StatefulSetLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ssLister = l
}

func (e *Engine) SetDaemonSetLister(l appsv1lister.DaemonSetLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dsLister = l
}

func (e *Engine) SetServiceLister(l corev1lister.ServiceLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.serviceLister = l
}

func (e *Engine) SetAuditLogger(l *audit.AuditLogger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.auditLogger = l
}

// SnapshotAll returns a deep copy of all non-resolved incidents keyed by
// incident key. It clears the dirty flag, so callers that must observe the
// state without consuming the dirty flag (e.g. the mass-failure hook, which
// may run alongside other SnapshotAll consumers) should use ActiveIncidents.
func (e *Engine) SnapshotAll() map[model.IncidentKey]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.dirty {
		return nil
	}
	out := make(map[model.IncidentKey]*model.Incident, len(e.state))
	for key, inc := range e.state {
		if inc.State == model.StateResolved {
			continue
		}
		out[key] = inc.Clone()
	}
	e.dirty = false
	return out
}

// ActiveIncidents returns a deep copy of every non-resolved incident keyed by
// key, without touching the dirty flag. It is safe to call from multiple
// consumers within a single lifecycle tick.
func (e *Engine) ActiveIncidents() map[model.IncidentKey]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[model.IncidentKey]*model.Incident, len(e.state))
	for key, inc := range e.state {
		if inc.State == model.StateResolved {
			continue
		}
		out[key] = inc.Clone()
	}
	return out
}

// SnapshotPersisted returns all non-resolved incidents in their serializable
// form so the controller can persist them across restarts.
func (e *Engine) SnapshotPersisted() []model.PersistedIncident {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotPersistedLocked()
}

// FreezeAndSnapshotPersisted stops runtime incident changes and returns the
// final serializable state. It is only for shutdown: freezing is permanent.
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

// RestoreIncidents loads previously persisted incidents into the state map.
// Only incidents whose key still exists in the seen (baseline) set are
// restored, to avoid re-alerting for issues that were resolved while down.
// LastSeen is bumped to now to prevent immediate cleanup-loop resolution.
// Mass-failure incidents (mass-failure/<dependency> keys) are restored into
// the dedicated massFailures store regardless of the baseline so an ongoing
// mass failure is not re-alerted after a restart.
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
			clone := inc.Clone()
			if clone.Fingerprint == "" {
				clone.Fingerprint = legacyFingerprint(clone.Key)
			}
			clone.LastSeen = now
			clone.LastUpdate = now
			clone.NotifiedSig = notifSig(clone)
			e.massFailures[key] = clone
			restored++
			continue
		}
		restoreKey := key
		if _, ok := e.baseline[string(restoreKey)]; !ok {
			if migrated, ok := e.migrateLegacyPodKey(key, inc); ok {
				restoreKey = migrated
			}
		}
		if _, ok := e.baseline[string(restoreKey)]; !ok ||
			len(e.baseline[string(restoreKey)]) == 0 {
			continue
		}
		if _, exists := e.state[restoreKey]; exists {
			continue
		}
		clone := inc.Clone()
		clone.Key = restoreKey
		if clone.Fingerprint == "" {
			clone.Fingerprint = legacyFingerprint(clone.Key)
		}
		clone.LastSeen = now
		clone.LastUpdate = now
		clone.NotifiedSig = notifSig(clone)
		e.state[restoreKey] = clone
		e.indexIncidentByNamespace(clone)
		restored++
	}
	if restored > 0 {
		klog.InfoS("restored incidents from ConfigMap", "count", restored)
	}
}

// MassFailureSet returns a snapshot of the currently tracked mass-failure
// incidents keyed by their incident key.
func (e *Engine) MassFailureSet() map[model.IncidentKey]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[model.IncidentKey]*model.Incident, len(e.massFailures))
	for key, inc := range e.massFailures {
		out[key] = inc.Clone()
	}
	return out
}

// HasMassFailure reports whether a mass-failure incident with the given key is
// already being tracked (used to avoid duplicate alerts).
func (e *Engine) HasMassFailure(key model.IncidentKey) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.hasMassFailureLocked(key)
}

// hasMassFailureLocked is the lock-free form for callers already inside the
// engine mutex. Process holds it for its whole body, and sync.Mutex is not
// reentrant.
func (e *Engine) hasMassFailureLocked(key model.IncidentKey) bool {
	_, ok := e.massFailures[key]
	return ok
}

// AddMassFailure registers a synthetic mass-failure incident and announces it.
// It is persisted so a restart does not re-alert. Returns true if it was newly
// tracked; a duplicate is neither stored nor announced. Callers must not hold
// e.mu.
func (e *Engine) AddMassFailure(inc *model.Incident) bool {
	e.mu.Lock()
	if e.frozen || inc == nil {
		e.mu.Unlock()
		return false
	}
	key := inc.Key
	if _, exists := e.massFailures[key]; exists {
		e.mu.Unlock()
		return false
	}
	stored := inc.Clone()
	stored.FirstSeen = e.now()
	stored.LastSeen = stored.FirstSeen
	if stored.State != model.StateResolved {
		stored.State = model.StateActive
	}
	stored.NotifiedSig = notifSig(stored)
	e.massFailures[key] = stored
	e.dirty = true
	snap := stored.Clone()
	e.mu.Unlock()

	e.emit(transition{snap, model.ActionCreate})
	return true
}

// RemoveMassFailure resolves a tracked mass failure: it announces the resolve
// and then releases every incident the mass failure was speaking for, so a
// symptom that outlived its cause is announced instead of lost. Returns true
// if the mass failure was tracked at all. Callers must not hold e.mu.
func (e *Engine) RemoveMassFailure(key model.IncidentKey) bool {
	e.mu.Lock()
	if e.frozen {
		e.mu.Unlock()
		return false
	}
	inc, exists := e.massFailures[key]
	if !exists {
		e.mu.Unlock()
		return false
	}
	delete(e.massFailures, key)
	resolved := inc.Clone()
	resolved.State = model.StateResolved
	resolved.NotifiedSig = notifSig(resolved)
	released := e.releaseSuppressedLocked(key)
	e.mu.Unlock()

	e.emit(
		append([]transition{{resolved, model.ActionResolved}}, released...)...)
	return true
}

func (e *Engine) SetSeverityMap(m map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if en, ok := e.config.Enricher.(*enricher.DefaultEnricher); ok {
		en.SetSeverityMap(m)
	}
}
