package correlation

import (
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/model"
)

// flushGroupBuffers emits or updates a smart-group incident per buffer whose
// grouping window has elapsed. Caller must hold e.mu.
func (e *Engine) flushGroupBuffers(now time.Time) []transition {
	var pending []transition
	for gk, pg := range e.groupBuffers {
		if len(pg.entries) == 0 || !now.After(pg.firstSeen.Add(e.config.SmartGroupingWindow)) {
			continue
		}
		if t, ok := e.flushOneGroup(gk, pg, now); ok {
			pending = append(pending, t)
		}
		delete(e.groupBuffers, gk)
	}
	return pending
}

// flushOneGroup materializes the synthetic incident for a single exhausted
// group buffer, tracking its members for a later batch resolve. Caller must
// hold e.mu. The bool reports whether a transition should be emitted.
func (e *Engine) flushOneGroup(gk string, pg *pendingGroup, now time.Time) (transition, bool) {
	var active []groupEntry
	for _, ge := range pg.entries {
		if inc, ok := e.state[ge.key]; ok && inc.State != model.StateResolved && inc.State != model.StatePendingResolve {
			active = append(active, ge)
		}
	}
	if len(active) == 0 {
		return transition{}, false
	}

	groupInc := e.buildGroupIncident(gk, pg, active, now)
	summary := groupInc.Hint
	groupIncKey := groupInc.Key

	// Update-not-create: once a group has been notified, re-flushes carry the
	// same stable key and emit an UPDATE, throttled by a cooldown so a busy
	// group can't spam every flush window.
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
	// Replace any stale tracker for the same key, then track group members so
	// a later flush can still batch resolve. The tracker is maintained on
	// every flush path — including the cooldown-suppressed skip — so a
	// sustained group keeps a live batch-resolve handle.
	delete(e.groupResolveTrackers, gk)
	e.trackGroupResolve(gk, pg, active, summary, groupIncKey, now)

	// Reset NotifiedSig on active entries so subsequent events can be
	// re-grouped. This must also happen on the skip path: without it, member
	// incidents keep a non-empty NotifiedSig and tryGroupIncident refuses to
	// re-buffer them, so the group stops emitting its recurring UPDATE once
	// the renotify cooldown expires.
	for _, ge := range active {
		if inc, ok := e.state[ge.key]; ok {
			inc.NotifiedSig = ""
		}
	}

	if action == model.ActionSkip {
		return transition{}, false
	}
	return transition{groupInc, action}, true
}

// buildGroupIncident assembles the synthetic incident for a flushed group,
// copying rich data (logs, events, runbook) from the first member.
// Caller must hold e.mu.
func (e *Engine) buildGroupIncident(gk string, pg *pendingGroup, active []groupEntry, now time.Time) *model.Incident {
	summary := e.buildGroupSummary(active, pg.firstSeen)
	if pg.overflowCount > 0 {
		summary += fmt.Sprintf(" +%d more", pg.overflowCount)
	}
	// Stable key per group so re-flushes update the same incident instead of
	// creating a new one each cycle.
	groupIncKey := model.IncidentKey(groupKeyPrefix + gk)
	sev := e.groupSeverity(active)
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
		e.carryGroupMemberData(groupInc, mem, summary)
	}
	return groupInc
}

// carryGroupMemberData forwards actionable diagnostics from a group member
// incident to the group notification. Caller must hold e.mu.
func (e *Engine) carryGroupMemberData(groupInc, mem *model.Incident, summary string) {
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

// trackGroupResolve records the members of a flushed group so a later batch
// resolve can resolve the whole group at once. Caller must hold e.mu.
func (e *Engine) trackGroupResolve(gk string, pg *pendingGroup, active []groupEntry, summary string, groupIncKey model.IncidentKey, now time.Time) {
	sev := e.groupSeverity(active)
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
}
