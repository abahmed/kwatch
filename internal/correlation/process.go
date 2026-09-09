package correlation

import (
	"fmt"
	"hash/crc32"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// Process runs one observation through the engine and announces whatever it
// decides. It is the engine's only entry point for a finding.
//
// The returned incident is a copy and the action is the decision that was (or
// was not) announced; callers do not notify on their own — the engine already
// did, through the lifecycle hook, so the live path and every timer-driven
// path share one audit, one diagnosis and one delivery.
//
// A nil observation is "nothing found" and is skipped, so a detector's result
// can be handed straight here without a nil check at every call site.
func (e *Engine) Process(obs *model.Observation) (
	*model.Incident, model.IncidentAction,
) {
	if obs == nil {
		return nil, model.ActionSkip
	}
	return e.processEvent(
		event.FromObservation(obs), obs.OwnerPath(), obs.State(),
	)
}

// processEvent is Process once the observation has been rendered as an event.
// The engine's internals are written against the event, which is also what
// providers receive.
func (e *Engine) processEvent(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) (*model.Incident, model.IncidentAction) {
	// Signature extraction scans the whole log tail against 35 regexes and
	// the Service lookup lists a namespace; both depend only on the event, so
	// they are done before the lock rather than holding every other producer
	// behind them.
	pre := e.precompute(ev)
	e.mu.Lock()
	if e.frozen {
		e.mu.Unlock()
		return nil, model.ActionSkip
	}
	inc, action := e.processLocked(ev, owner, cs, pre)
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
//  3. identity     — which incident is this? dedup, silent revival,
//     crash-loop key fold, escalation. The cooldown lives inside this stage: a
//     recurrence it covers still updates its incident, it just does not
//     speak, so a resolve never ping-pongs with a re-create and a still
//     broken workload is never silently forgotten.
//  4. announcement — should it speak now, wait for its group, or stay quiet
//     because nothing observable changed? See grouping.go and edgeAction.
//
// Caller must hold e.mu.
func (e *Engine) processLocked(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
	pre precomputed,
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
		return e.recordSymptom(c, ev, owner, cs, key, res, now, pre)
	}
	// 3. identity
	//
	// The pod-UID bookkeeping is recorded on the paths that keep an
	// incident, and only those. Recording it up front meant every
	// cooldown-skipped report added a UID for a key with no incident behind
	// it, and nothing ever removed those.
	if inc, ok := e.state[key]; ok {
		e.rememberPodResource(key, ev)
		// Already resolved — revive instead of re-creating. Re-creating
		// would emit a CREATE notification, causing a
		// resolved→CREATE→resolved flip-flop cycle. Reviving keeps the
		// existing incident, its thread and its counters.
		if inc.State == model.StateResolved ||
			inc.State == model.StatePendingResolve {
			// Inside the cleanup cooldown the revival is silent: the resolve
			// it follows is minutes old, and announcing again so soon is the
			// flip-flop the cooldown exists to prevent. Outside it, the
			// recurrence is news and speaks as an update.
			silent := e.inCooldown(key)
			if silent {
				e.auditSkipOnce(key, ev, "cooldown")
			}
			return e.refreshIncident(inc, ev, cs, owner, now, silent)
		}

		if e.config.EscalationEnabled && cs != nil &&
			e.escalateRestartCount(inc, ev, cs, now) {
			return inc, e.edgeAction(inc)
		}
		return e.refreshIncident(inc, ev, cs, owner, now, false)
	}

	// No incident left to revive, so the cooldown does suppress: creating one
	// now would open a second thread for a problem whose resolve was just
	// announced.
	if e.skipByCooldown(key, ev) {
		return nil, model.ActionSkip
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
		if orphan, ok := e.foldCrashLoopIncident(ev, key, res); ok {
			e.rememberPodResource(key, ev)
			return e.refreshIncident(orphan, ev, cs, owner, now, false)
		}
	}

	inc := e.newIncident(ev, owner, cs, key, res, now, pre)
	e.state[key] = inc
	e.rememberPodResource(key, ev)
	// The key is alerting again, so a later suppression is newsworthy.
	e.clearSkipAudit(key)
	e.indexIncident(inc)

	// 4. announcement: buffer into a group, or speak on the edge.
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
	pre precomputed,
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
			Transient:   ev.Transient,
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
	if owner == "" ||
		(ev.Resource == "pod" && ev.OwnerKind == "" && owner == ev.PodName) {
		inc.Name = ev.PodName
	}
	// Identity is settled here, once, from the name as it stands. Later
	// passes may rewrite the name as replicas are replaced; they must not
	// move the incident to a different subject.
	inc.SetObject()
	if ev.OwnerKind != "" {
		inc.Owner = model.ObjectRef{
			Kind:      ev.OwnerKind,
			Namespace: ev.Namespace,
			Name:      owner,
		}
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	if url, ok := e.config.Runbooks[ev.Reason]; ok {
		inc.Runbook = url
	}
	e.applyIncidentEscalation(&ev, inc, cs)
	e.config.Enricher.Enrich(&ev, inc)

	// Topology is impact, not explanation. It used to be appended to the
	// hint as prose, where it repeated what the diagnosis block already says
	// and made the hint unreadable. Keep it structured; renderers decide how
	// to show it.
	e.attachIncidentRelations(inc, owner, res, pre)

	return inc
}

// precomputed holds the work Process does before taking the engine lock.
type precomputed struct {
	services []string
}

// precompute resolves everything that depends only on the event. Callers must
// NOT hold e.mu; the lister reference is taken under a short lock so this
// cannot race with SetServiceLister.
func (e *Engine) precompute(ev event.Event) precomputed {
	e.mu.Lock()
	lister := e.serviceLister
	e.mu.Unlock()
	return precomputed{
		services: dependentServices(lister, ev.Namespace, ev.Labels),
	}
}

func (e *Engine) applyIncidentEscalation(
	ev *event.Event,
	inc *model.Incident,
	cs *model.ContainerState,
) {
	if !e.config.EscalationEnabled || cs == nil {
		return
	}
	cur := int(cs.RestartCount)
	if tier := crossedTier(-1, cur, e.config.EscalationTiers); tier >= 0 {
		ev.Severity = severityForTier(tier, inc.Severity)
		return
	}
	if ev.Severity != "" {
		return
	}
	for i := len(e.config.EscalationTiers) - 1; i >= 0; i-- {
		if cur >= e.config.EscalationTiers[i] {
			ev.Severity = severityForTier(i, inc.Severity)
			break
		}
	}
}

func (e *Engine) attachIncidentRelations(
	inc *model.Incident,
	owner, res string,
	pre precomputed,
) {
	if deps := pre.services; len(deps) > 0 {
		sort.Strings(deps)
		inc.AffectedServices = deps
	}
	if res == "pod" && owner != "" && !e.isOwnerHealthy(inc) {
		inc.OwnerUnhealthy = true
	}
}
