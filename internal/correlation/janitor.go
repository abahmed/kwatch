package correlation

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// transition is a deferred notification for a lifecycle change.
type transition struct {
	inc    *model.Incident
	action model.IncidentAction
}

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
		if !now.After(inc.LastSeen.Add(e.config.Window)) {
			continue
		}
		// Do not clean up pod incidents whose owning workload is still unhealthy.
		if !e.isOwnerHealthy(inc) {
			continue
		}
		// Finalize active/digested/pending-resolve incidents with a resolve
		// so the LifecycleHook emits a resolved notification and Slack's
		// threadMap is pruned. StatePendingResolve that outlives the stale
		// window is finalized here too — otherwise the incident would be
		// silently dropped without a resolved notification.
		if inc.State != model.StateResolved {
			inc.State = model.StateResolved
			// Smart group batch resolve
			if groupInc, groupAction, tracked := e.tryConsumeGroupResolve(key); tracked {
				if groupAction != model.ActionSkip {
					pending = append(pending, transition{groupInc, groupAction})
				}
			} else if a := e.edgeAction(inc); a != model.ActionSkip {
				pending = append(pending, transition{inc.Clone(), a})
			}
		}
		// Add cooldown to prevent resolve→recreate cycle for still-broken resources
		e.cleanupCooldown[key] = now.Add(e.config.Window)
		e.removeBaselineForIncident(key, inc)
		e.removeIncidentFromNamespaceIndex(inc)
		delete(e.state, key)
		if inc.Resource == "node" {
			e.refreshNodeInhibition(inc.Name)
		}
	}
	e.mu.Unlock()
	for _, t := range pending {
		if h := e.config.LifecycleHook; h != nil {
			h(t.inc, t.action)
		}
	}
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

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
		}
	}
	if hook := e.config.MassFailureHook; hook != nil {
		hook()
	}
	if baselineChanged {
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.baseline)
			e.mu.Unlock()
			hook(snapshot)
		}
	}
}

// finalizePendingResolves resolves incidents whose resolve hold-down has
// expired and the owning workload is healthy again. Caller must hold e.mu.
func (e *Engine) finalizePendingResolves(now time.Time) ([]transition, bool) {
	var pending []transition
	var baselineChanged bool
	for key, inc := range e.state {
		if inc.State != model.StatePendingResolve || inc.ResolveAt.IsZero() || !now.After(inc.ResolveAt) {
			continue
		}
		// Do not finalize if the owning workload is still unhealthy.
		if !e.isOwnerHealthy(inc) {
			inc.State = model.StateActive
			inc.ResolveAt = time.Time{}
			continue
		}
		inc.State = model.StateResolved
		if inc.Resource == "node" {
			e.refreshNodeInhibition(inc.Name)
		}
		e.removeBaselineForIncident(key, inc)
		e.cleanupCooldown[key] = now.Add(e.config.Window)
		// Smart group batch resolve
		if groupInc, groupAction, tracked := e.tryConsumeGroupResolve(key); tracked {
			baselineChanged = true
			if groupAction != model.ActionSkip {
				pending = append(pending, transition{groupInc, groupAction})
			}
		} else {
			action := e.edgeAction(inc)
			baselineChanged = true
			pending = append(pending, transition{inc.Clone(), action})
		}
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
		if inc.State == model.StateResolved || inc.State == model.StatePendingResolve {
			continue
		}
		if grouped[inc.Key] {
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
			pending = append(pending, transition{inc.Clone(), model.ActionUpdate})
		}
	}
	return pending
}
