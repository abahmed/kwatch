package correlation

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) StartCleanup(ctx context.Context) {
	cleanupInterval := e.config.Window / 2
	if cleanupInterval < 30*time.Second {
		cleanupInterval = 30 * time.Second
	}
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	lifecycleTicker := time.NewTicker(e.config.LifecycleInterval)
	defer lifecycleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("correlation cleanup stopped")
			return
		case <-cleanupTicker.C:
			e.cleanup()
		case <-lifecycleTicker.C:
			e.checkLifecycle()
		}
	}
}

func (e *Engine) cleanup() {
	e.mu.Lock()
	e.dirty = true
	now := e.now()
	var pending []transition
	for key, inc := range e.state {
		if now.Before(inc.LastSeen.Add(e.config.Window)) {
			continue
		}
		// Do not clean up pod incidents whose owning workload is still
		// unhealthy.
		if !e.isOwnerHealthy(inc) {
			continue
		}
		// Finalize active/pending-resolve incidents with a resolve so the
		// LifecycleHook emits a resolved notification and Slack's threadMap
		// is pruned. StatePendingResolve that outlives the stale window is
		// finalized here too — otherwise the incident would be silently
		// dropped without a resolved notification.
		if inc.State != model.StateResolved {
			pending = append(pending, e.resolveLocked(key, inc, now))
		} else {
			// Already resolved: still arm the cooldown so a recurrence of a
			// still-broken resource revives silently.
			e.cleanupCooldown[key] = now.Add(e.config.Window)
			e.removeBaselineForIncident(key, inc)
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Name)
			}
		}
		e.removeIncidentFromNamespaceIndex(inc)
		delete(e.state, key)
		e.clearSkipAudit(key)
	}
	e.mu.Unlock()
	e.emit(pending...)
}

func (e *Engine) checkLifecycle() {
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	now := e.now()

	// pending resolve finalization
	pending, baselineChanged = e.finalizePendingResolves(now)

	// renotify — resend on time-based interval (not stale-gated).
	// Incidents absorbed into a smart group are skipped: the group flush
	// (and its cooldown-gated re-flush) is their re-notification channel,
	// so individual renotify would duplicate the group alert.
	pending = append(pending, e.renotifyDue(now)...)

	// smart grouping flush
	if e.config.SmartGroupingWindow > 0 {
		pending = append(pending, e.flushGroupBuffers(now)...)
	}

	e.mu.Unlock()

	e.emit(pending...)
	if hook := e.config.MassFailureHook; hook != nil {
		hook()
	}
	if baselineChanged {
		e.publishBaseline()
	}
}

// finalizePendingResolves resolves incidents whose resolve hold-down has
// expired and the owning workload is healthy again. Caller must hold e.mu.
func (e *Engine) finalizePendingResolves(now time.Time) ([]transition, bool) {
	var pending []transition
	var baselineChanged bool
	for key, inc := range e.state {
		if inc.State != model.StatePendingResolve || inc.ResolveAt.IsZero() ||
			now.Before(inc.ResolveAt) {
			continue
		}
		// Do not finalize if the owning workload is still unhealthy.
		if !e.isOwnerHealthy(inc) {
			inc.State = model.StateActive
			inc.ResolveAt = time.Time{}
			continue
		}
		baselineChanged = true
		pending = append(pending, e.resolveLocked(key, inc, now))
	}
	return pending, baselineChanged
}

// renotifyDue collects update notifications for active incidents whose
// per-severity renotify interval has elapsed. Caller must hold e.mu.
func (e *Engine) renotifyDue(now time.Time) []transition {
	var pending []transition
	renotifyBySev := e.config.RenotifyIntervalBySeverity
	if len(renotifyBySev) == 0 {
		return pending
	}
	grouped := e.groupedKeys()
	maxPer := e.config.RenotifyMaxPerIncident
	if maxPer <= 0 {
		maxPer = 3
	}
	for _, inc := range e.state {
		if inc.State == model.StateResolved ||
			inc.State == model.StatePendingResolve {
			continue
		}
		if grouped[inc.Key] || inc.SuppressedBy != "" {
			continue
		}
		if inc.RenotifyCount >= maxPer {
			continue
		}
		interval, ok := renotifyBySev[string(inc.Severity)]
		if !ok || interval <= 0 {
			interval, ok = renotifyBySev["default"]
		}
		if !ok || interval <= 0 {
			continue
		}
		if now.After(inc.LastNotifiedAt.Add(interval)) {
			inc.RenotifyCount++
			inc.LastNotifiedAt = now
			// For renotify we emit update
			pending = append(
				pending,
				transition{inc.Clone(), model.ActionUpdate},
			)
		}
	}
	return pending
}
