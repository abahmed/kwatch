package correlation

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// refreshIncident applies the latest event to an existing incident, keeping
// it active and returning the notification action. It covers three cases with
// identical bookkeeping: reviving a resolved incident (avoids the
// resolved→CREATE→resolved flip-flop that re-creating would cause), revoking
// a pending resolve, and a routine update to an already-active incident.
//
// silent asks for the bookkeeping without the announcement, which is what a
// revival inside the cleanup cooldown wants: the incident carries on and
// keeps counting, and the next real change — an escalation, or its eventual
// resolve — is what speaks. Caller must hold e.mu.
func (e *Engine) refreshIncident(
	inc *model.Incident,
	ev event.Event,
	cs *model.ContainerState,
	owner string,
	now time.Time,
	silent bool,
) (*model.Incident, model.IncidentAction) {
	// A revival starts a fresh renotify budget. Otherwise an incident that
	// resolved after maxing out renotify would never be re-notified again
	// when the same problem recurs.
	revived := inc.State == model.StateResolved ||
		inc.State == model.StatePendingResolve
	inc.State = model.StateActive
	inc.ResolveAt = time.Time{}
	if revived {
		inc.RenotifyCount = 0
		inc.LastNotifiedAt = time.Time{}
	}
	addResource(inc, ev.PodName)
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.Containers[ev.ContainerName] = true
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	if ev.Resource == "pod" && ev.OwnerKind == "" && owner == ev.PodName {
		// Keep the display name current when a Pod replacement continues the
		// same explicit lineage. The correlation key is stable; the name is
		// intentionally human-facing and may change.
		inc.Name = ev.PodName
	}
	inc.Count++
	inc.LastSeen = now
	inc.LastUpdate = now
	e.config.Enricher.Enrich(&ev, inc)
	if silent {
		// Pre-recording the signature is what makes the announcement a
		// no-op. Grouping is skipped for the same reason: buffering into a
		// group would announce on the group's flush, which is exactly the
		// message the cooldown is suppressing. The renotify clock restarts
		// from here, so a problem that is still unfixed one interval later
		// does say so.
		inc.NotifiedSig = notifSig(inc)
		inc.LastNotifiedAt = now
		return inc, model.ActionSkip
	}
	if e.tryGroupIncident(inc, ev, owner, now) {
		return inc, model.ActionSkip
	}
	return inc, e.edgeAction(inc)
}

// resolveLocked finalises an incident as resolved and returns what to
// announce: the group's resolve once every member is done, the incident's own
// resolve on the edge, or a skip.
//
// Every path that ends an incident — markResolved, Resolve, the
// stale-incident cleanup, the hold-down finaliser — goes through here. Each
// used to carry its own copy of this bookkeeping, and they had drifted: one
// armed the cooldown during hold-down (so a recurrence inside the hold-down
// was swallowed and the incident resolved anyway), the others did not. One
// function means one behaviour. Caller must hold e.mu.
func (e *Engine) resolveLocked(
	key model.IncidentKey,
	inc *model.Incident,
	now time.Time,
) transition {
	inc.State = model.StateResolved
	// LastSeen is the end of the incident timeline. A resolve can be caused by
	// a clean status observation or by a stale/hold-down sweep, so retaining the
	// last failure timestamp makes the rendered resolution point misleading.
	inc.LastSeen = now
	inc.LastUpdate = now
	if inc.Resource == "node" {
		e.refreshNodeInhibition(inc.Ref().Name)
	}
	e.removeBaselineForIncident(key, inc)
	delete(e.podResourceUIDs, key)
	// Arm the cooldown so a recurrence within the window revives silently
	// instead of producing a resolved→CREATE→resolved flip-flop.
	e.cleanupCooldown[key] = now.Add(e.config.Window)
	// A group member's resolve is folded into the group's: one "all
	// recovered" message replaces N individual ones.
	if groupInc, groupAction, tracked := e.groupMemberResolved(key); tracked {
		return transition{groupInc, groupAction}
	}
	return transition{inc.Clone(), e.edgeAction(inc)}
}

// holdDownLocked parks a recovered incident until ResolveHoldDown elapses, so
// a blip that comes straight back never shows a fake "resolved". Node
// inhibition is lifted immediately: the condition has already recovered and
// only the announcement is delayed. The cooldown is NOT armed here — a
// recurrence during the hold-down must revive the incident, not be swallowed.
// Caller must hold e.mu.
func (e *Engine) holdDownLocked(inc *model.Incident, now time.Time) {
	inc.State = model.StatePendingResolve
	inc.ResolveAt = now.Add(e.config.ResolveHoldDown)
	if inc.Resource == "node" {
		e.refreshNodeInhibition(inc.Ref().Name)
	}
}

// markResolved closes one incident by its exact key. It is the engine's
// internal primitive: callers outside this package say which subject
// recovered (Resolve) or hand back the observation (ResolveObserved), and
// never spell a key.
func (e *Engine) markResolved(key model.IncidentKey) {
	e.mu.Lock()
	if e.frozen {
		e.mu.Unlock()
		return
	}
	e.dirty = true
	inc, ok := e.state[key]
	if !ok || inc.State == model.StateResolved ||
		inc.State == model.StatePendingResolve {
		// No incident to resolve, but the observation of recovery still
		// retires any owner-level baseline entry that was suppressing one.
		cleared := e.clearBaselineEntryForKey(key)
		e.mu.Unlock()
		if cleared {
			e.publishBaseline()
		}
		return
	}
	// Do not resolve if the owning workload is still unhealthy.
	if !e.isOwnerHealthy(inc) {
		e.mu.Unlock()
		return
	}
	now := e.now()
	if e.config.ResolveHoldDown > 0 {
		e.holdDownLocked(inc, now)
		e.mu.Unlock()
		return
	}
	t := e.resolveLocked(key, inc, now)
	e.mu.Unlock()

	e.emit(t)
	e.publishBaseline()
}

func (e *Engine) RemovePod(namespace, podName string) {
	e.RemovePodWithUID(namespace, podName, "")
}

func (e *Engine) RemovePodWithUID(namespace, podName, podUID string) {
	var baselineChanged bool

	e.mu.Lock()
	if e.frozen {
		e.mu.Unlock()
		return
	}
	e.dirty = true
	for key, inc := range e.state {
		if inc.Namespace != namespace {
			continue
		}
		if !inc.Resources[podName] {
			continue
		}
		if podUID != "" {
			// An identified delete is allowed to remove a resource only when
			// this incident recorded that exact UID. Unknown identity is
			// deliberately conservative: after a restart, deleting an old
			// tombstone must not remove a replacement from restored state.
			if refs := e.podResourceUIDs[key]; refs == nil || refs[podName] != podUID {
				continue
			}
		}
		delete(inc.Resources, podName)
		if refs := e.podResourceUIDs[key]; refs != nil {
			delete(refs, podName)
			if len(refs) == 0 {
				delete(e.podResourceUIDs, key)
			}
		}
		// Pod removal does NOT resolve incidents. During a crash loop, the
		// ReplicaSet replaces pods continuously and each deletion would
		// resolve the incident, then the new pod would re-create it, causing
		// a flip-flop cycle. Resolution is handled solely by cleanup(),
		// checkLifecycle(), and markResolved().
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
		e.publishBaseline()
	}
}
