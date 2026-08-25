package correlation

import (
	"strings"
	"time"

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
		// The node condition has already recovered; un-suppress pods now
		// instead of waiting for the hold-down to finalize.
		if inc.Resource == "node" {
			e.refreshNodeInhibition(inc.Name)
		}
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
				// The node condition has already recovered; un-suppress pods
				// now instead of waiting for the hold-down to finalize.
				if inc.Resource == "node" {
					e.refreshNodeInhibition(inc.Name)
				}
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
