package correlation

import (
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/model"
)

// flushOneGroup materializes the synthetic incident for a single exhausted
// group buffer, tracking its members for a later batch resolve. Caller must
// hold e.mu. The bool reports whether a transition should be emitted.
func (e *Engine) flushOneGroup(
	gk string,
	pg *pendingGroup,
	now time.Time,
) (transition, bool) {
	var active []groupEntry
	for _, ge := range pg.entries {
		if inc, ok := e.state[ge.key]; ok && inc.State != model.StateResolved &&
			inc.State != model.StatePendingResolve {
			active = append(active, ge)
		}
	}
	if len(active) == 0 {
		return transition{}, false
	}

	// A "group" of one is just an incident. Announcing it as a group costs a
	// full grouping window of latency and reads as "1 pod in ns/owner
	// (total 1)" while grouping nothing, so emit the member itself instead.
	// Only while the group has never been announced: once it has an identity
	// in the chat provider it must keep it, or that thread is orphaned with
	// no resolve.
	if _, announced := e.groupFlushStates[gk]; !announced && len(active) == 1 {
		mem, ok := e.state[active[0].key]
		if !ok {
			return transition{}, false
		}
		// Buffering overwrote NotifiedSig to hold the member back; restore the
		// real value so edgeAction can distinguish create from update.
		mem.NotifiedSig = active[0].prevNotifiedSig
		delete(e.groupResolveTrackers, gk)
		if a := e.edgeAction(mem); a != model.ActionSkip {
			return transition{mem.Clone(), a}, true
		}
		return transition{}, false
	}

	// The pending buffer is discarded after every flush, so pg.firstSeen is
	// only ever "when this window opened". A group that keeps re-flushing must
	// carry its original start time, otherwise each update reports a duration
	// of one grouping window and a problem broken for hours always reads ~1m.
	firstSeen := pg.firstSeen
	if fs, ok := e.groupFlushStates[gk]; ok && !fs.firstSeen.IsZero() {
		firstSeen = fs.firstSeen
	}

	groupInc := e.buildGroupIncident(gk, pg, active, now, firstSeen)
	summary := groupInc.Hint
	groupIncKey := groupInc.Key

	action := decideGroupFlush(
		e.groupFlushStates[gk],
		now,
		e.groupRenotifyCooldown(),
	)
	e.applyGroupFlush(gk, action, now, firstSeen)
	// Fold this wave's members into the group's tracker so a later flush can
	// still batch resolve. The tracker is maintained on every flush path --
	// including the cooldown-suppressed skip -- so a sustained group keeps a
	// live batch-resolve handle.
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
func (e *Engine) buildGroupIncident(
	gk string,
	pg *pendingGroup,
	active []groupEntry,
	now, firstSeen time.Time,
) *model.Incident {
	summary := e.buildGroupSummary(active)
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
		Subject: model.Subject{
			ID: fmt.Sprintf(
				"%08x",
				crc32.ChecksumIEEE([]byte(groupIncKey)),
			),
			Key:       groupIncKey,
			Reason:    active[0].reason,
			Name:      summary,
			Namespace: active[0].namespace,
			Resource:  active[0].kind,
		},
		Status: model.Status{
			Resources:     resources,
			PeakResources: len(resources),
			Count:         len(active),
			FirstSeen:     firstSeen,
			LastSeen:      now,
			Severity:      sev,
		},
		Evidence: model.Evidence{
			Hint: summary,
		},
	}

	if mem, ok := e.state[active[0].key]; ok {
		e.carryGroupMemberData(groupInc, mem, summary)
	}
	return groupInc
}

// carryGroupMemberData forwards actionable diagnostics from a group member
// incident to the group notification. Caller must hold e.mu.
func (e *Engine) carryGroupMemberData(
	groupInc, mem *model.Incident,
	summary string,
) {
	if mem.Hint != "" && !strings.Contains(mem.Hint, summary) {
		groupInc.Hint = enricher.CombineHints(groupInc.Hint, mem.Hint)
	}
	groupInc.Facts = mem.Facts
	groupInc.Logs = mem.Logs
	groupInc.IncludeLogs = mem.IncludeLogs
	groupInc.Events = mem.Events
	groupInc.EvidencePod = mem.EvidencePod
	groupInc.AffectedServices = append([]string(nil), mem.AffectedServices...)
	groupInc.OwnerUnhealthy = mem.OwnerUnhealthy
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
//
// Members accumulate across flushes. A group that fills over several windows
// -- forty HPAs failing in the two minutes after a cold start -- flushes in
// waves, and replacing the tracker each time kept only the last wave: every
// earlier member then resolved on its own, forty green ticks for one event.
// Members that have left the engine's state are dropped so the group cannot
// wait forever on an incident that no longer exists.
func (e *Engine) trackGroupResolve(
	gk string,
	pg *pendingGroup,
	active []groupEntry,
	summary string,
	groupIncKey model.IncidentKey,
	now time.Time,
) {
	tracker := e.groupResolveTrackers[gk]
	if tracker == nil || tracker.groupIncKey != groupIncKey {
		tracker = &groupResolveTracker{
			groupIncKey: groupIncKey,
			members:     make(map[model.IncidentKey]bool),
			firstSeen:   pg.firstSeen,
		}
		e.groupResolveTrackers[gk] = tracker
	}
	for key := range tracker.members {
		if inc, ok := e.state[key]; !ok ||
			inc.State == model.StateResolved {
			delete(tracker.members, key)
		}
	}
	for _, ge := range active {
		if _, tracked := tracker.members[ge.key]; !tracked {
			tracker.members[ge.key] = false
		}
	}
	tracker.totalCount = len(tracker.members)
	tracker.summary = summary
	tracker.reason = active[0].reason
	tracker.lastSeen = now
	tracker.severity = e.groupSeverity(active)
}

// decideGroupFlush is the notification decision for one group flush, as a
// function of what was already sent.
//
// Update-not-create: once a group has been notified, re-flushes carry the
// same stable key and emit an UPDATE, throttled by a cooldown so a busy group
// cannot spam every flush window. The rule used to be spelled as writes
// interleaved with the branches that decided them, which is why it could only
// be read by tracing the mutations. Pure, so it can be read -- and later
// tested -- on its own.
func decideGroupFlush(
	fs *groupFlushState,
	now time.Time,
	cooldown time.Duration,
) model.IncidentAction {
	if fs == nil || !fs.notified {
		return model.ActionCreate
	}
	if now.After(fs.lastNotifiedAt.Add(cooldown)) {
		return model.ActionUpdate
	}
	return model.ActionSkip
}

// applyGroupFlush records the decision decideGroupFlush made. A skip records
// nothing: the group keeps the timestamp of the notification that is still
// inside its cooldown. Caller must hold e.mu.
func (e *Engine) applyGroupFlush(
	gk string,
	action model.IncidentAction,
	now, firstSeen time.Time,
) {
	switch action {
	case model.ActionCreate:
		e.groupFlushStates[gk] = &groupFlushState{
			notified:       true,
			lastNotifiedAt: now,
			firstSeen:      firstSeen,
		}
	case model.ActionUpdate:
		if fs := e.groupFlushStates[gk]; fs != nil {
			fs.lastNotifiedAt = now
		}
	}
}
