package correlation

import (
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// skipByBaseline reports whether the event is covered by the startup baseline
// and should not create an incident. Node events are never baselined so the
// incident is always created. Caller must hold e.mu.
func (e *Engine) skipByBaseline(res string, key model.IncidentKey, ev event.Event) bool {
	if res == "node" || !e.isBaselined(key, ev.PodName) {
		return false
	}
	if e.auditLogger != nil {
		e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: string(key)}, "baseline")
	}
	return true
}

// suppressedByNode reports whether the event should be suppressed because a
// node incident is already inhibiting pod alerts. Caller must hold e.mu.
func (e *Engine) suppressedByNode(ev event.Event, owner string, key model.IncidentKey, res string) bool {
	if !e.config.InhibitNodeSuppressesPods || res != "pod" {
		return false
	}
	return e.suppressedByNodeIncident(ev, owner, key)
}

// skipByCooldown reports whether re-creation is suppressed by the cleanup
// cooldown, cleaning up expired entries. Caller must hold e.mu.
func (e *Engine) skipByCooldown(key model.IncidentKey, ev event.Event) bool {
	expiry, ok := e.cleanupCooldown[key]
	if !ok {
		return false
	}
	if !e.now().Before(expiry) {
		delete(e.cleanupCooldown, key)
		return false
	}
	if e.auditLogger != nil {
		e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: string(key)}, "cooldown")
	}
	return true
}

// suppressCascading suppresses a pod incident whose owning workload already
// has an active non-pod incident, attributing the pod to the workload
// incident instead. Caller must hold e.mu.
func (e *Engine) suppressCascading(ev event.Event, owner string, key model.IncidentKey, now time.Time) bool {
	ownerFull := OwnerPath(ev.Namespace, owner)
	for _, existing := range e.state {
		if existing.State == model.StateResolved ||
			existing.State == model.StatePendingResolve {
			continue
		}
		if existing.Resource == "pod" ||
			existing.Namespace != ev.Namespace {
			continue
		}
		// The owner must match in the incident's Name field and in the
		// owner slot of its key — the key check guards against two
		// distinct owners sharing one namespace prefix.
		pk := ParseKey(existing.Key)
		if existing.Name != owner && existing.Name != ownerFull {
			continue
		}
		if pk.Owner != owner && pk.Owner != ownerFull {
			continue
		}
		existing.Count++
		if ev.PodName != "" {
			existing.Resources[ev.PodName] = true
			if len(existing.Resources) > existing.PeakResources {
				existing.PeakResources = len(existing.Resources)
			}
		}
		existing.LastSeen = now
		if e.auditLogger != nil {
			e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: string(key)}, "cascading_suppression")
		}
		return true
	}
	return false
}
