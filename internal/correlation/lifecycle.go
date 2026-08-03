package correlation

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// refreshIncident applies the latest event to an existing incident, keeping
// it active and returning the notification action. It covers three cases with
// identical bookkeeping: silently reviving a resolved incident (avoids the
// resolved→CREATE→resolved flip-flop that re-creating would cause), revoking
// a pending resolve, and a routine update to an already-active incident.
// Caller must hold e.mu.
func (e *Engine) refreshIncident(inc *model.Incident, ev event.Event, cs *model.ContainerState, owner string, now time.Time) (*model.Incident, model.IncidentAction) {
	// A revival starts a fresh renotify budget. Otherwise an incident that
	// resolved after maxing out renotify would never be re-notified again
	// when the same problem recurs.
	revived := inc.State == model.StateResolved || inc.State == model.StatePendingResolve
	inc.State = model.StateActive
	inc.ResolveAt = time.Time{}
	if revived {
		inc.RenotifyCount = 0
		inc.LastNotifiedAt = time.Time{}
	}
	if ev.PodName != "" {
		inc.Resources[ev.PodName] = true
		if len(inc.Resources) > inc.PeakResources {
			inc.PeakResources = len(inc.Resources)
		}
	}
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.Containers[ev.ContainerName] = true
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	inc.Count++
	inc.LastSeen = now
	inc.LastUpdate = now
	e.config.Enricher.Enrich(&ev, inc)
	if e.tryGroupIncident(inc, ev, owner, now) {
		return inc, model.ActionSkip
	}
	return inc, e.edgeAction(inc)
}

func (e *Engine) MarkResolved(key model.IncidentKey) {
	e.mu.Lock()
	e.dirty = true
	inc, ok := e.state[key]
	if !ok || inc.State == model.StateResolved || inc.State == model.StatePendingResolve {
		e.mu.Unlock()
		return
	}
	// Do not resolve if the owning workload is still unhealthy.
	if !e.isOwnerHealthy(inc) {
		e.mu.Unlock()
		return
	}
	if e.config.ResolveHoldDown > 0 {
		inc.State = model.StatePendingResolve
		inc.ResolveAt = e.now().Add(e.config.ResolveHoldDown)
		e.mu.Unlock()
		return
	}
	// Smart group batch resolve: check if this incident is a member of a
	// tracked smart group. If so, buffer the resolve and only emit one
	// notification when all members have resolved.
	if groupInc, groupAction, tracked := e.tryConsumeGroupResolve(key); tracked {
		inc.State = model.StateResolved
		if inc.Resource == "node" {
			e.refreshNodeInhibition(inc.Name)
		}
		e.removeBaselineForIncident(key, inc)
		e.cleanupCooldown[key] = e.now().Add(e.config.Window)
		e.mu.Unlock()
		if groupAction != model.ActionSkip {
			if hook := e.config.LifecycleHook; hook != nil {
				hook(groupInc.Clone(), groupAction)
			}
		}
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.baseline)
			e.mu.Unlock()
			hook(snapshot)
		}
		return
	}
	inc.State = model.StateResolved
	if inc.Resource == "node" {
		e.refreshNodeInhibition(inc.Name)
	}
	e.removeBaselineForIncident(key, inc)
	// Arm the cooldown so a recurrence within the window is suppressed
	// (preventing a resolved→CREATE→resolved flip-flop), then revives
	// silently once the cooldown expires.
	e.cleanupCooldown[key] = e.now().Add(e.config.Window)
	action := e.edgeAction(inc)
	snap := inc.Clone()
	e.mu.Unlock()

	if action != model.ActionSkip {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(snap, action)
		}
	}
	if hook := e.config.OnBaselineChange; hook != nil {
		e.mu.Lock()
		snapshot := cloneBaseline(e.baseline)
		e.mu.Unlock()
		hook(snapshot)
	}
}

func (e *Engine) RemovePod(namespace, podName string) {
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	for _, inc := range e.state {
		if inc.Namespace != namespace {
			continue
		}
		if !inc.Resources[podName] {
			continue
		}
		delete(inc.Resources, podName)
		// Pod removal does NOT resolve incidents. During a crash loop, the
		// ReplicaSet replaces pods continuously and each deletion would
		// resolve the incident, then the new pod would re-create it, causing
		// a flip-flop cycle. Resolution is handled solely by cleanup(),
		// checkLifecycle(), and MarkResolved().
	}
	// Release per-pod baseline slots for this pod, scoped to the namespace
	// so an identically-named pod in another namespace keeps its baseline.
	nsPrefix := namespace + ":"
	for key, pods := range e.baseline {
		if !strings.HasPrefix(key, nsPrefix) {
			continue
		}
		if _, ok := pods[podName]; ok {
			delete(pods, podName)
			baselineChanged = true
			if len(pods) == 0 {
				delete(e.baseline, key)
			}
		}
	}
	// Evict all per-container state entries for this pod.
	podPrefix := namespace + "/" + podName + "/"
	for k := range e.lastContainerIndex {
		if strings.HasPrefix(k, podPrefix) {
			delete(e.lastContainerIndex, k)
		}
	}
	e.mu.Unlock()

	if baselineChanged {
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.baseline)
			e.mu.Unlock()
			hook(snapshot)
		}
	}
}

func (e *Engine) ResolveByResource(resource, name string) {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	now := e.now()
	for key, inc := range e.state {
		if inc.Resource == resource && inc.Name == name && inc.State != model.StateResolved {
			if inc.State == model.StatePendingResolve {
				continue
			}
			// For pod incidents owned by a workload, gate on workload health.
			if !e.isOwnerHealthy(inc) {
				continue
			}
			if e.config.ResolveHoldDown > 0 {
				inc.State = model.StatePendingResolve
				inc.ResolveAt = now.Add(e.config.ResolveHoldDown)
				e.cleanupCooldown[key] = now.Add(e.config.Window)
				continue
			}
			inc.State = model.StateResolved
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Name)
			}
			e.removeBaselineForIncident(key, inc)
			e.cleanupCooldown[key] = now.Add(e.config.Window)
			// Smart group batch resolve: when the incident is a group member,
			// the group RESOLVED replaces the individual notification.
			if groupInc, groupAction, tracked := e.groupMemberResolved(key); tracked {
				if groupAction != model.ActionSkip {
					pending = append(pending, transition{groupInc, groupAction})
				}
			} else if action := e.edgeAction(inc); action != model.ActionSkip {
				baselineChanged = true
				pending = append(pending, transition{inc.Clone(), action})
			}
		}
	}
	e.mu.Unlock()

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
		}
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
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
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
	// Drain group resolves deferred from Process (e.g. orphan folding) where
	// the lock prevented a synchronous hook call.
	for _, gi := range e.deferredResolves {
		pending = append(pending, transition{gi, model.ActionResolved})
	}
	e.deferredResolves = nil
	e.mu.Unlock()
	for _, t := range pending {
		if h := e.config.LifecycleHook; h != nil {
			h(t.inc, t.action)
		}
	}
}

func (e *Engine) checkLifecycle() {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	now := e.now()

	// pending resolve finalization
	for key, inc := range e.state {
		if inc.State == model.StatePendingResolve && !inc.ResolveAt.IsZero() && now.After(inc.ResolveAt) {
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
	}

	// renotify — resend on time-based interval (not stale-gated).
	// Incidents absorbed into a smart group are skipped: the group flush
	// (and its cooldown-gated re-flush) is their re-notification channel,
	// so individual renotify would duplicate the group alert.
	renotifyBySev := e.config.RenotifyIntervalBySeverity
	if len(renotifyBySev) > 0 {
		grouped := e.groupedKeys()
		for _, inc := range e.state {
			if inc.State == model.StateResolved || inc.State == model.StatePendingResolve {
				continue
			}
			if grouped[inc.Key] {
				continue
			}
			maxPer := e.config.RenotifyMaxPerIncident
			if maxPer <= 0 {
				maxPer = 3
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
	}

	// smart grouping flush
	if e.config.SmartGroupingWindow > 0 {
		for gk, pg := range e.groupBuffers {
			if len(pg.entries) > 0 && now.After(pg.firstSeen.Add(e.config.SmartGroupingWindow)) {
				var active []groupEntry
				for _, ge := range pg.entries {
					if inc, ok := e.state[ge.key]; ok && inc.State != model.StateResolved && inc.State != model.StatePendingResolve {
						active = append(active, ge)
					}
				}
				if len(active) > 0 {
					summary := e.buildGroupSummary(active, pg.firstSeen)
					if pg.overflowCount > 0 {
						summary += fmt.Sprintf(" +%d more", pg.overflowCount)
					}
					// Stable key per group so re-flushes update the same
					// incident instead of creating a new one each cycle.
					groupIncKey := model.IncidentKey(groupKeyPrefix + gk)
					sev := e.groupSeverity(active)
					// Copy rich data (logs, events, analysis, runbook) from the first
					// member incident so the group notification includes diagnostics.
					resources := make(map[string]bool)
					for _, ge := range active {
						if ge.podName != "" {
							resources[ge.podName] = true
						}
					}
					groupInc := &model.Incident{
						ID:            fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(groupIncKey))),
						Key:           groupIncKey,
						Reason:        active[0].reason,
						Name:          summary,
						Namespace:     active[0].namespace,
						Resource:      active[0].kind,
						Resources:     resources,
						PeakResources: len(resources),
						Count:         len(active),
						FirstSeen:     pg.firstSeen,
						LastSeen:      now,
						Hint:          summary,
						Severity:      sev,
					}
					if mem, ok := e.state[active[0].key]; ok {
						// Carry forward rich diagnostic data from the first member
						// so the group notification includes actionable details
						// (memory limits for OOM, log signatures, etc.).
						if mem.Hint != "" && !strings.Contains(mem.Hint, summary) {
							groupInc.Hint = enricher.CombineHints(groupInc.Hint, mem.Hint)
						}
						groupInc.Logs = mem.Logs
						groupInc.IncludeLogs = mem.IncludeLogs
						groupInc.Events = mem.Events
						groupInc.IncludeEvents = mem.IncludeEvents
						groupInc.ContainerName = mem.ContainerName
						groupInc.OwnerKind = mem.OwnerKind
						groupInc.Runbook = mem.Runbook
						groupInc.Analysis = mem.Analysis
						groupInc.Image = mem.Image
						groupInc.NodeName = mem.NodeName
						groupInc.RestartCount = mem.RestartCount
						if mem.LastContainerState != nil {
							cs := *mem.LastContainerState
							groupInc.LastContainerState = &cs
						}
						groupInc.Containers = make(map[string]bool)
						for c := range mem.Containers {
							groupInc.Containers[c] = true
						}
					}
					// Update-not-create: once a group has been notified, re-flushes
					// carry the same stable key and emit an UPDATE, throttled by a
					// cooldown so a busy group can't spam every flush window.
					action := model.ActionCreate
					if fs, ok := e.groupFlushStates[gk]; ok && fs.notified {
						if now.After(fs.lastNotifiedAt.Add(e.groupRenotifyCooldown())) {
							fs.lastNotifiedAt = now
							action = model.ActionUpdate
						} else {
							action = model.ActionSkip
						}
					} else {
						e.groupFlushStates[gk] = &groupFlushState{
							notified:       true,
							lastNotifiedAt: now,
						}
					}
					if action != model.ActionSkip {
						pending = append(pending, transition{groupInc, action})
					}

					// Replace any stale tracker for the same key
					delete(e.groupResolveTrackers, gk)

					// Track group members for batch resolve
					tracker := &groupResolveTracker{
						groupIncKey: groupIncKey,
						members:     make(map[model.IncidentKey]bool),
						totalCount:  len(active),
						summary:     summary,
						reason:      active[0].reason,
						firstSeen:   pg.firstSeen,
						lastSeen:    now,
						severity:    sev,
					}
					for _, ge := range active {
						tracker.members[ge.key] = false
					}
					e.groupResolveTrackers[gk] = tracker

					// Reset NotifiedSig on active entries so subsequent events can be re-grouped
					for _, ge := range active {
						if inc, ok := e.state[ge.key]; ok {
							inc.NotifiedSig = ""
						}
					}
				}
				delete(e.groupBuffers, gk)
			}
		}
	}

	// Drain group resolves deferred from Process (e.g. orphan folding) where
	// the lock prevented a synchronous hook call.
	for _, gi := range e.deferredResolves {
		pending = append(pending, transition{gi, model.ActionResolved})
	}
	e.deferredResolves = nil

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
