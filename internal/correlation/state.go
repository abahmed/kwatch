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
		e.namespaceIndex[ns] = make(map[string]*model.Incident)
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

func (e *Engine) SetAnalysis(key, analysis string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inc, ok := e.state[key]; ok {
		inc.Analysis = analysis
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

// SnapshotAll returns a deep copy of all non-resolved incidents keyed by ID.
func (e *Engine) SnapshotAll() map[string]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.dirty {
		return nil
	}
	out := make(map[string]*model.Incident, len(e.state))
	for key, inc := range e.state {
		if inc.State == model.StateResolved {
			continue
		}
		out[key] = inc.Clone()
	}
	e.dirty = false
	return out
}

// SnapshotPersisted returns all non-resolved incidents in their serializable
// form so the controller can persist them across restarts.
func (e *Engine) SnapshotPersisted() []model.PersistedIncident {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.PersistedIncident, 0, len(e.state))
	for _, inc := range e.state {
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
func (e *Engine) RestoreIncidents(incidents map[string]*model.Incident) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	if len(incidents) == 0 {
		return
	}
	now := e.now()
	restored := 0
	for key, inc := range incidents {
		if _, ok := e.seen[key]; !ok || len(e.seen[key]) == 0 {
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

func (e *Engine) SetSeverityMap(m map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if en, ok := e.config.Enricher.(*enricher.DefaultEnricher); ok {
		en.SetSeverityMap(m)
	}
}
