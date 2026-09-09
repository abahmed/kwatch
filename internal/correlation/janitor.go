package correlation

import (
	"context"
	"strings"
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

// staleResolveNote marks a resolve that came from the stale sweep instead of
// an observation. Kept short: it is appended to an existing hint.
const staleResolveNote = "closed after no further reports; " +
	"the underlying problem was not observed to recover"

// staleGraceWindows caps how long an incident may be held open purely because
// its object still exists. Without a cap a permanently wedged object would be
// remembered forever, which is the unbounded-memory failure this sweep exists
// to prevent; with it, presence buys the incident four windows of patience and
// no more.
const staleGraceWindows = 4

func staleResolveHint(hint string) string {
	if hint == "" {
		return staleResolveNote
	}
	if strings.Contains(hint, staleResolveNote) {
		return hint
	}
	return hint + " — " + staleResolveNote
}

func (e *Engine) cleanup() {
	e.mu.Lock()
	e.dirty = true
	now := e.now()
	e.removeExpiredCooldowns(now)
	e.pruneContainerStates(now)
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
		// Nor close one whose object is demonstrably still there: silence is
		// not recovery.
		if e.subjectStillPresent(inc, now) {
			continue
		}
		// Finalize active/pending-resolve incidents with a resolve so the
		// LifecycleHook emits a resolved notification and Slack's threadMap
		// is pruned. StatePendingResolve that outlives the stale window is
		// finalized here too — otherwise the incident would be silently
		// dropped without a resolved notification.
		if inc.State != model.StateResolved {
			// This is not an observed recovery: the object simply stopped
			// being reported for a whole Window. Say so, rather than
			// presenting it as "fixed".
			inc.Hint = staleResolveHint(inc.Hint)
			pending = append(pending, e.resolveLocked(key, inc, now))
		} else {
			// Already resolved and now aged out. The cooldown is NOT
			// re-armed: there is no incident left to revive silently, so
			// re-arming only bought a second window of silence for a
			// resource that may still be broken, and the report that finally
			// got through arrived as a brand-new incident on a new thread.
			e.removeBaselineForIncident(key, inc)
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Ref().Name)
			}
		}
		e.unindexIncident(inc)
		delete(e.state, key)
		// The UID bookkeeping outlives its incident otherwise: resolveLocked
		// clears it on the resolve path, and nothing cleared it here.
		delete(e.podResourceUIDs, key)
		e.clearSkipAudit(key)
	}
	e.mu.Unlock()
	e.emit(pending...)
}

// removeExpiredCooldowns bounds the per-incident cooldown index even when a
// historical key never appears again. Caller must hold e.mu.
func (e *Engine) removeExpiredCooldowns(now time.Time) {
	for key, expiry := range e.cleanupCooldown {
		if !now.Before(expiry) {
			delete(e.cleanupCooldown, key)
			e.clearSkipAudit(key)
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

// subjectStillPresent reports whether a stale incident should be held open
// because the object it is about still exists.
//
// It answers false when there is no presence check, when the check cannot
// speak for that kind, when the incident already reached a resolve of its own,
// or once the grace cap has run out — in each of those cases the caller falls
// back to closing on staleness. Caller must hold e.mu.
func (e *Engine) subjectStillPresent(
	inc *model.Incident,
	now time.Time,
) bool {
	if e.config.SubjectPresent == nil || inc == nil {
		return false
	}
	// A recovery that was actually observed, or a hold-down already running,
	// is not a guess and must not be delayed.
	if inc.State != model.StateActive {
		return false
	}
	// An incident reported by a Kubernetes Event is about something that
	// happened, not about a state the object is in. That the object still
	// exists is no evidence the problem persists, so presence buys it no
	// patience: it closes on silence like any other unreported incident.
	if inc.Transient {
		return false
	}
	grace := e.config.Window * staleGraceWindows
	if !now.Before(inc.LastSeen.Add(grace)) {
		return false
	}
	return e.anyObjectPresent(inc)
}

// maxPresenceChecks bounds the cache lookups one incident may cost per sweep.
// A wide incident can name hundreds of pods, and one surviving pod is enough
// to answer the question.
const maxPresenceChecks = 16

// anyObjectPresent reports whether any object the incident is about is still
// in the cache.
//
// The incident's own Name is not usable for this: on a Pod incident it holds
// the *owning workload's* name, so looking up a Pod by it always says
// NotFound and the check silently does nothing for the most common kind of
// incident there is. model.ObjectRefs owns that rule.
func (e *Engine) anyObjectPresent(inc *model.Incident) bool {
	refs := inc.ObjectRefs()
	if len(refs) > maxPresenceChecks {
		refs = refs[:maxPresenceChecks]
	}
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		exists, known := e.config.SubjectPresent(
			ref.Kind,
			ref.Namespace,
			ref.Name,
		)
		if known && exists {
			return true
		}
	}
	return false
}
