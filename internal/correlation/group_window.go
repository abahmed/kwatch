package correlation

import (
	"fmt"
	"hash/crc32"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

// groupRenotifyCooldown returns the minimum interval between notifications
// for the same smart group. Re-flushes within the interval silently refresh
// the group's state without notifying, so a busy group can't spam updates on
// every flush window.
func (e *Engine) groupRenotifyCooldown() time.Duration {
	w := e.config.SmartGroupingWindow
	cd := 4 * w
	if cd < 5*time.Minute {
		cd = 5 * time.Minute
	}
	if cd > 30*time.Minute {
		cd = 30 * time.Minute
	}
	return cd
}

// groupedKeys returns the set of incident keys currently absorbed into a
// smart group (buffered in a pending group or tracked as a flushed group
// member). Renotify skips these — the group notification is the single
// re-notification channel for grouped incidents, so per-member renotify
// would duplicate the group alert. Caller must hold e.mu.
func (e *Engine) groupedKeys() map[model.IncidentKey]bool {
	keys := make(map[model.IncidentKey]bool)
	for _, pg := range e.groupBuffers {
		for _, ge := range pg.entries {
			keys[ge.key] = true
		}
	}
	for _, tracker := range e.groupResolveTrackers {
		for k := range tracker.members {
			keys[k] = true
		}
	}
	return keys
}

// rekeyGroupReferences moves any smart-group membership for an incident from
// oldKey to newKey. Used when the crash-loop fold re-keys an incident so the
// group keeps tracking it (otherwise the group would wait forever on a member
// that no longer exists). Caller must hold e.mu.
func (e *Engine) rekeyGroupReferences(oldKey, newKey model.IncidentKey) {
	for _, pg := range e.groupBuffers {
		for i := range pg.entries {
			if pg.entries[i].key == oldKey {
				pg.entries[i].key = newKey
			}
		}
	}
	for _, tracker := range e.groupResolveTrackers {
		if _, ok := tracker.members[oldKey]; ok {
			delete(tracker.members, oldKey)
			tracker.members[newKey] = false
		}
	}
}

// Caller must hold e.mu.
// tryGroupIncident attempts to add an event to the smart grouping buffer.
// Returns true if the incident was grouped (caller should return ActionSkip).
func (e *Engine) tryGroupIncident(
	inc *model.Incident,
	ev event.Event,
	owner string,
	now time.Time,
) bool {
	if e.config.SmartGroupingWindow <= 0 || inc.NotifiedSig != "" {
		return false
	}
	plan := planGroupEntry(inc, ev, owner)
	if e.firstOwnerInWindow(
		plan.key,
		plan.entry.reason,
		ev.Namespace,
		owner,
		inc.Key,
		now,
	) {
		return false // announce now; nothing to group yet
	}
	e.bufferGroupEntry(plan, now)
	inc.NotifiedSig = notifSig(inc)
	inc.LastNotifiedAt = now
	metrics.DefaultRegistry().IncidentsGrouped.Add(1)
	return true
}

// bufferGroupEntry is the effect half of grouping: it files the planned entry
// under its group, holding the buffer to its size bound. Caller must hold
// e.mu.
func (e *Engine) bufferGroupEntry(plan groupPlan, now time.Time) {
	pg, ok := e.groupBuffers[plan.key]
	if !ok {
		pg = &pendingGroup{firstSeen: now}
		e.groupBuffers[plan.key] = pg
	}
	pg.entries = append(pg.entries, plan.entry)
	if len(pg.entries) > maxGroupEntries {
		pg.entries = pg.entries[1:]
		pg.overflowCount++
	}
}

// ownerScopeOf reports the reason|namespace scope when gk is an owner-scoped
// group key for this event, and "" otherwise. Node, image and signature keys
// are three-part too, so the shape is checked against the event, not counted.
func ownerScopeOf(gk, r, namespace, owner string) string {
	if namespace == "" || owner == "" ||
		gk != ownerGroupKey(r, namespace, owner) {
		return ""
	}
	return r + "|" + namespace
}

// firstOwnerInWindow records the owner in its reason|namespace window and
// reports whether it is the first one there — in which case the caller
// announces the incident immediately instead of buffering it. Caller must hold
// e.mu.
func (e *Engine) firstOwnerInWindow(
	gk, r, namespace, owner string,
	key model.IncidentKey,
	now time.Time,
) bool {
	scope := ownerScopeOf(gk, r, namespace, owner)
	if scope == "" {
		return false
	}
	w := e.fanOutWindows[scope]
	if w == nil || now.Sub(w.firstSeen) > e.config.SmartGroupingWindow {
		w = &ownerWindow{
			firstSeen: now,
			owners:    map[string]bool{},
			announced: map[string]model.IncidentKey{},
		}
		e.fanOutWindows[scope] = w
	}
	w.owners[owner] = true
	if len(w.owners) == 1 {
		w.announced[owner] = key
		return true
	}
	return false
}

// pruneFanOutWindows drops owner windows that have closed. Caller must hold
// e.mu.
func (e *Engine) pruneFanOutWindows(now time.Time) {
	for scope, w := range e.fanOutWindows {
		if now.Sub(w.firstSeen) > e.config.SmartGroupingWindow {
			delete(e.fanOutWindows, scope)
		}
	}
}

// groupMemberResolved marks key as resolved within its group tracker and
// returns the group's resolved notification once every member has resolved.
// Caller must hold e.mu. tracked reports whether key belonged to a group.
// Members removed from state outside markResolved (orphan folding, resource
// resolution) must go through here so the group isn't left waiting forever
// on a member that no longer exists.
func (e *Engine) groupMemberResolved(
	key model.IncidentKey,
) (groupInc *model.Incident, action model.IncidentAction, tracked bool) {
	for gk, tracker := range e.groupResolveTrackers {
		if _, ok := tracker.members[key]; ok {
			tracker.members[key] = true
			allResolved := true
			for _, resolved := range tracker.members {
				if !resolved {
					allResolved = false
					break
				}
			}
			if !allResolved {
				return nil, model.ActionSkip, true
			}
			e.closeGroupTracker(gk)
			return tracker.resolvedIncident(), model.ActionResolved, true
		}
	}
	return nil, model.ActionSkip, false
}

// closeGroupTracker forgets a finished group. The flush state goes with the
// tracker so a genuinely new occurrence of the same group creates a fresh
// incident -- a stable-key UPDATE after RESOLVED would otherwise re-open a
// closed incident. Caller must hold e.mu.
func (e *Engine) closeGroupTracker(gk string) {
	delete(e.groupResolveTrackers, gk)
	delete(e.groupFlushStates, gk)
}

// resolvedIncident renders the tracker as the group's resolved notification.
func (t *groupResolveTracker) resolvedIncident() *model.Incident {
	return &model.Incident{
		Subject: model.Subject{
			ID: fmt.Sprintf(
				"%08x",
				crc32.ChecksumIEEE([]byte(t.groupIncKey)),
			),
			Key:    t.groupIncKey,
			Reason: t.reason,
			Name:   t.summary,
		},
		Status: model.Status{
			Count:     t.totalCount,
			FirstSeen: t.firstSeen,
			LastSeen:  t.lastSeen,
			State:     model.StateResolved,
			Severity:  t.severity,
		},
		Evidence: model.Evidence{
			Hint: t.summary,
		},
	}
}

// pruneGroupState closes groups that can no longer complete on their own.
//
// A tracker is only forgotten when every member resolves through
// groupMemberResolved. Any path that drops a member from state without going
// through it -- and any member that is simply never heard from again --
// leaves the tracker waiting forever: the group is never resolved, the
// channel never sees it close, and the tracker and its flush state stay in
// memory for the life of the process. The wait ends when no member is left to
// wait for -- every one is either resolved or gone from state.
//
// Age deliberately does not end it. A group is the notification channel for
// its members, so closing one whose members are still live would hand those
// members back to individual renotify and undo the grouping. The members
// themselves are bounded by the stale sweep, which is what bounds the tracker.
//
// Groups closed here still emit a resolved notification -- the alternative is
// deleting the state silently, which is exactly the "problem still open in
// Slack forever" symptom. Caller must hold e.mu.
func (e *Engine) pruneGroupState(now time.Time) []transition {
	var pending []transition
	grace := e.config.Window * staleGraceWindows
	for gk, tracker := range e.groupResolveTrackers {
		if e.groupCanStillComplete(tracker) {
			continue
		}
		e.closeGroupTracker(gk)
		pending = append(pending, transition{
			inc:    tracker.resolvedIncident(),
			action: model.ActionResolved,
		})
	}
	// A flush state with no tracker and no buffer belongs to a group that is
	// finished; keeping it only risks suppressing a future occurrence.
	for gk, st := range e.groupFlushStates {
		if _, tracked := e.groupResolveTrackers[gk]; tracked {
			continue
		}
		if _, buffered := e.groupBuffers[gk]; buffered {
			continue
		}
		if now.Before(st.lastNotifiedAt.Add(grace)) {
			continue
		}
		delete(e.groupFlushStates, gk)
	}
	return pending
}

// groupCanStillComplete reports whether any member is both unresolved and
// still present in state, i.e. whether the group has anyone left to wait for.
// Caller must hold e.mu.
func (e *Engine) groupCanStillComplete(t *groupResolveTracker) bool {
	for key, resolved := range t.members {
		if resolved {
			continue
		}
		if _, live := e.state[key]; live {
			return true
		}
	}
	return false
}
