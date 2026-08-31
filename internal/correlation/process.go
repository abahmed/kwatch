package correlation

import (
	"fmt"
	"hash/crc32"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// Process runs one observed event through the engine and announces whatever
// it decides. The returned incident is a copy and the action is the decision
// that was (or was not) announced; callers do not notify on their own — the
// engine already did, through the lifecycle hook, so the live path and every
// timer-driven path share one audit, one diagnosis and one delivery.
func (e *Engine) Process(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) (*model.Incident, model.IncidentAction) {
	e.mu.Lock()
	inc, action := e.processLocked(ev, owner, cs)
	if inc != nil {
		inc = inc.Clone()
	}
	e.mu.Unlock()

	e.emit(transition{inc, action})
	return inc, action
}

// processLocked is the decision half of Process. Every event walks the same
// five stages in the same order:
//
//  1. baseline     — pre-existing at startup: never news.
//  2. attribution  — a symptom of a known cause (node, shared dependency,
//     owning workload): recorded against it, not announced. See attribution.go.
//  3. cooldown     — recently resolved and still broken: silent until the
//     window ends, so a resolve never ping-pongs with a re-create.
//  4. identity     — which incident is this? dedup, silent revival, crash-loop
//     key fold, escalation.
//  5. announcement — should it speak now, wait for its group, or stay quiet
//     because nothing observable changed? See grouping.go and edgeAction.
//
// Caller must hold e.mu.
func (e *Engine) processLocked(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) (*model.Incident, model.IncidentAction) {
	e.dirty = true

	key := IncidentKey(ev, owner, cs)

	res := ev.Resource
	if res == "" {
		res = "pod"
	}

	// Node events feed the inhibition map before anything can gate them, so
	// a baselined node still explains the pods on it.
	e.trackNodeIncident(res, ev.NodeName, ev.Reason)

	// 1. baseline
	if e.skipByBaseline(res, key, ev) {
		return nil, model.ActionSkip
	}

	now := e.now()

	// 2. attribution
	if c := e.attribute(ev, owner, key, res); c.kind != causeNone {
		return e.recordSymptom(c, ev, owner, cs, key, res, now)
	}
	e.rememberPodResource(key, ev)

	// 3. cooldown
	if e.skipByCooldown(key, ev) {
		return nil, model.ActionSkip
	}

	// 4. identity
	if inc, ok := e.state[key]; ok {
		// Already resolved — silently revive instead of re-creating.
		// Re-creating would emit a CREATE notification, causing a
		// resolved→CREATE→resolved flip-flop cycle. Silent revival
		// keeps the existing incident active and returns ActionUpdate.
		if inc.State == model.StateResolved ||
			inc.State == model.StatePendingResolve {
			return e.refreshIncident(inc, ev, cs, owner, now)
		}

		if e.config.EscalationEnabled && cs != nil &&
			e.escalateRestartCount(inc, ev, cs, now) {
			return inc, e.edgeAction(inc)
		}
		return e.refreshIncident(inc, ev, cs, owner, now)
	}

	// When RestartCount crosses the CrashLoopHighFrequency threshold, the
	// incident key changes from the raw reason (e.g. constant.ReasonError) to
	// constant.ReasonCrashLoopHighFreq. Rather than orphaning the old incident
	// (which silently dropped it without a RESOLVED and fired a duplicate
	// CREATE for the same ongoing crash loop), migrate the existing incident
	// to the folded key: same ID (alert thread continuity), same FirstSeen,
	// no notification churn. Baseline entries are carried over so a startup-
	// baselined loop stays suppressed after the fold.
	if cs != nil && int(cs.RestartCount) > defaultCrashLoopHighFreqThreshold {
		if orphan, ok := e.foldCrashLoopIncident(ev, owner, key, res); ok {
			return e.refreshIncident(orphan, ev, cs, owner, now)
		}
	}

	inc := e.newIncident(ev, owner, cs, key, res, now)
	e.state[key] = inc
	// The key is alerting again, so a later suppression is newsworthy.
	e.clearSkipAudit(key)
	e.indexIncidentByNamespace(inc)

	// 5. announcement: buffer into a group, or speak on the edge.
	if e.tryGroupIncident(inc, ev, owner, now) {
		return inc, model.ActionSkip
	}

	return inc, e.edgeAction(inc)
}

// Caller must hold e.mu.
func (e *Engine) newIncident(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
	key model.IncidentKey,
	res string,
	now time.Time,
) *model.Incident {
	inc := &model.Incident{
		Subject: model.Subject{
			ID:          fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(key))),
			Fingerprint: StableFingerprint(ev, owner, cs),
			Key:         key,
			Reason:      ev.Reason,
			Namespace:   ev.Namespace,
			Resource:    res,
			Name:        owner,
			NodeName:    ev.NodeName,
		},
		Status: model.Status{
			Count:      1,
			FirstSeen:  now,
			LastSeen:   now,
			LastUpdate: now,
			State:      model.StateActive,
			Resources:  map[string]bool{},
			Containers: map[string]bool{},
		},
	}

	if ev.PodName != "" {
		inc.Resources[ev.PodName] = true
	}
	inc.PeakResources = len(inc.Resources)
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.Containers[ev.ContainerName] = true
	}
	// Keep the human-facing incident name as the concrete Pod name even when
	// correlation uses a UID or explicit lineage internally.
	if owner == "" || (ev.Resource == "pod" && ev.OwnerKind == "" && owner == ev.PodName) {
		inc.Name = ev.PodName
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	if url, ok := e.config.Runbooks[ev.Reason]; ok {
		inc.Runbook = url
	}
	if e.config.EscalationEnabled && cs != nil {
		cur := int(cs.RestartCount)
		if t := crossedTier(-1, cur, e.config.EscalationTiers); t >= 0 {
			ev.Severity = severityForTier(t, inc.Severity)
		} else if ev.Severity == "" {
			// seed from the absolute threshold when no tier is crossed at
			// startup
			for i := len(e.config.EscalationTiers) - 1; i >= 0; i-- {
				if cur >= e.config.EscalationTiers[i] {
					ev.Severity = severityForTier(i, inc.Severity)
					break
				}
			}
		}
	}
	e.config.Enricher.Enrich(&ev, inc)

	// Topology is impact, not explanation. It used to be appended to the
	// hint as prose, where it repeated what the diagnosis block already says
	// and made the hint unreadable. Keep it structured; renderers decide how
	// to show it.
	if deps := e.findDependentServices(ev.Namespace, ev.Labels); len(deps) > 0 {
		sort.Strings(deps)
		inc.AffectedServices = deps
	}
	if res == "pod" && owner != "" && !e.isOwnerHealthy(inc) {
		inc.OwnerUnhealthy = true
	}

	return inc
}
