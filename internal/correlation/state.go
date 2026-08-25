package correlation

import (
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

func (e *Engine) GetLastContainerState(namespace, podName, container string) *model.ContainerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	cs, ok := e.lastContainerIndex[lastContainerKey(namespace, podName, container)]
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
func (e *Engine) indexLastContainerState(namespace, podName, container string, cs *model.ContainerState) {
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
	out := make([]model.PersistedIncident, 0, len(e.state)+len(e.massFailures))
	for _, inc := range e.state {
		if inc.State == model.StateResolved {
			continue
		}
		out = append(out, inc.ToPersisted())
	}
	for _, inc := range e.massFailures {
		if inc.State == model.StateResolved {
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
func (e *Engine) RestoreIncidents(incidents map[model.IncidentKey]*model.Incident) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	if len(incidents) == 0 {
		return
	}
	now := e.now()
	restored := 0
	for key, inc := range incidents {
		if IsMassFailureKey(key) {
			if _, exists := e.massFailures[key]; exists {
				continue
			}
			inc.LastSeen = now
			inc.LastUpdate = now
			inc.NotifiedSig = notifSig(inc)
			e.massFailures[key] = inc
			restored++
			continue
		}
		if _, ok := e.baseline[string(key)]; !ok || len(e.baseline[string(key)]) == 0 {
			continue
		}
		if _, exists := e.state[key]; exists {
			continue
		}
		inc.LastSeen = now
		inc.LastUpdate = now
		inc.NotifiedSig = notifSig(inc)
		e.state[key] = inc
		e.indexIncidentByNamespace(inc)
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
	_, ok := e.massFailures[key]
	return ok
}

// AddMassFailure registers a synthetic mass-failure incident so it survives
// restarts without re-alerting. Returns true if it was newly tracked.
func (e *Engine) AddMassFailure(inc *model.Incident) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.massFailures[inc.Key]; exists {
		return false
	}
	inc.FirstSeen = e.now()
	inc.LastSeen = inc.FirstSeen
	e.massFailures[inc.Key] = inc
	e.dirty = true
	return true
}

// RemoveMassFailure drops a previously tracked mass-failure incident. Returns
// true if it was tracked at all.
func (e *Engine) RemoveMassFailure(key model.IncidentKey) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.massFailures[key]; !exists {
		return false
	}
	delete(e.massFailures, key)
	e.dirty = true
	return true
}

func (e *Engine) SetSeverityMap(m map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if en, ok := e.config.Enricher.(*enricher.DefaultEnricher); ok {
		en.SetSeverityMap(m)
	}
}
