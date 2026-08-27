package correlation

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// auditSkipOnce records a suppression the first time it happens for a key, and
// again only if the reason changes. Suppression is a steady state, not an
// event: re-recording it on every poll turns the audit log into a loop and
// buries everything else.
func (e *Engine) auditSkipOnce(
	key model.IncidentKey,
	ev event.Event,
	skipReason string,
) {
	if e.auditLogger == nil {
		return
	}
	if prev, ok := e.loggedSkips[key]; ok && prev == skipReason {
		return
	}
	e.loggedSkips[key] = skipReason
	e.auditLogger.LogSkip(&model.Incident{
		Subject: model.Subject{
			Key:       key,
			Namespace: ev.Namespace,
			Reason:    ev.Reason,
			NodeName:  ev.NodeName,
			ID:        string(key),
		},
	},

		skipReason)
}

// clearSkipAudit forgets a key's suppression state so the next suppression is
// audited again. Called when a key stops being suppressed.
func (e *Engine) clearSkipAudit(key model.IncidentKey) {
	delete(e.loggedSkips, key)
}

// skipByBaseline reports whether the event is covered by the startup baseline
// and should not create an incident. Node events are never baselined so the
// incident is always created. Caller must hold e.mu.
func (e *Engine) skipByBaseline(
	res string,
	key model.IncidentKey,
	ev event.Event,
) bool {
	if res == "node" || !e.isBaselined(key, ev.PodName) {
		e.clearSkipAudit(key)
		return false
	}
	e.auditSkipOnce(key, ev, "baseline")
	return true
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
		e.clearSkipAudit(key)
		return false
	}
	e.auditSkipOnce(key, ev, "cooldown")
	return true
}
