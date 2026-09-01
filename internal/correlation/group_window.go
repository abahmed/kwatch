package correlation

import (
	"fmt"
	"hash/crc32"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
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
	r := normalizeReason(ev.Reason)
	gk := computeGroupKey(r, ev, owner)
	if e.firstOwnerInWindow(gk, r, ev.Namespace, owner, inc.Key, now) {
		return false // announce now; nothing to group yet
	}
	pg, ok := e.groupBuffers[gk]
	if !ok {
		pg = &pendingGroup{firstSeen: now}
		e.groupBuffers[gk] = pg
	}
	sig := ""
	if r == constant.ReasonCrashLoopBackOff || r == constant.ReasonBackOff ||
		r == constant.ReasonError {
		sig = enricher.SignatureHint(ev.Logs)
	}
	entry := groupEntry{
		key:             inc.Key,
		prevNotifiedSig: inc.NotifiedSig,
		namespace:       ev.Namespace,
		owner:           owner,
		reason:          r,
		kind:            ev.Resource,
		podName:         ev.PodName,
		containerName:   ev.ContainerName,
		image:           ev.Image,
		nodeName:        ev.NodeName,
		logSignature:    sig,
	}
	pg.entries = append(pg.entries, entry)
	if len(pg.entries) > maxGroupEntries {
		pg.entries = pg.entries[1:]
		pg.overflowCount++
	}
	inc.NotifiedSig = notifSig(inc)
	inc.LastNotifiedAt = now
	metrics.DefaultRegistry().IncidentsGrouped.Add(1)
	return true
}

// ownerScopeOf reports the reason|namespace scope when gk is an owner-scoped
// group key for this event, and "" otherwise. Node, image and signature keys
// are three-part too, so the shape is checked against the event, not counted.
func ownerScopeOf(gk, r, namespace, owner string) string {
	if namespace == "" || owner == "" || gk != r+"|"+namespace+"|"+owner {
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
// Members removed from state outside MarkResolved (orphan folding, resource
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
			delete(e.groupResolveTrackers, gk)
			// Reset the flush state so a genuinely new occurrence of the
			// same group creates a fresh incident (a stable-key UPDATE
			// after RESOLVED would otherwise re-open a closed incident).
			delete(e.groupFlushStates, gk)
			groupInc := &model.Incident{
				Subject: model.Subject{
					ID: fmt.Sprintf(
						"%08x",
						crc32.ChecksumIEEE([]byte(tracker.groupIncKey)),
					),
					Key:    tracker.groupIncKey,
					Reason: tracker.reason,
					Name:   tracker.summary,
				},
				Status: model.Status{
					Count:     tracker.totalCount,
					FirstSeen: tracker.firstSeen,
					LastSeen:  tracker.lastSeen,
					State:     model.StateResolved,
					Severity:  tracker.severity,
				},
				Evidence: model.Evidence{
					Hint: tracker.summary,
				},
			}

			return groupInc, model.ActionResolved, true
		}
	}
	return nil, model.ActionSkip, false
}
