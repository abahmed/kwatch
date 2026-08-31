package correlation

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// Attribution answers one question about an event before anything else is
// decided: is this a symptom of something kwatch already knows about?
//
// Three kinds of cause are recognised, checked in this order — the broadest
// explanation wins:
//
//  1. a node-level condition on the pod's node (or, for a pod with no node,
//     any active node condition — it probably cannot schedule because of it);
//  2. a mass failure on a shared dependency the resource depends on;
//  3. an incident on the pod's own owning workload (a stuck rollout, a
//     failing Job).
//
// A symptom is recorded against its cause — counted, listed, kept — and is
// not announced on its own. The cause's alert speaks for it.
//
// This used to be five checks spread through Process, each with its own idea
// of "related" and its own bookkeeping. attribute is read-only and
// recordSymptom holds every side effect, so the precedence is visible in one
// place and a new kind of cause has exactly one place to go.

type causeKind int

const (
	causeNone causeKind = iota
	// the pod's node has an active node-level incident
	causeNodeCondition
	// a mass failure on a dependency already speaks for it
	causeSharedDependency
	// the owning workload has its own active incident
	causeOwnerWorkload
)

// auditReason is the skip reason written to the audit log for each cause.
// The strings are stable: they are documented and people grep for them.
func (k causeKind) auditReason() string {
	switch k {
	case causeNodeCondition:
		return "node_inhibition"
	case causeSharedDependency:
		return "mass_failure"
	case causeOwnerWorkload:
		return "cascading_suppression"
	}
	return ""
}

// cause is what an event was attributed to. Exactly one of the payload fields
// is meaningful for a given kind.
type cause struct {
	kind causeKind
	// node is the node incident to credit for causeNodeCondition. It is nil
	// when the node is inhibiting from the startup baseline alone and never
	// had an incident of its own.
	node *model.Incident
	// mass is the mass-failure incident key for causeSharedDependency.
	mass model.IncidentKey
	// owner is the workload incident for causeOwnerWorkload.
	owner *model.Incident
}

// attribute decides, without side effects, what the event is a symptom of.
// Caller must hold e.mu.
func (e *Engine) attribute(
	ev event.Event,
	owner string,
	key model.IncidentKey,
	res string,
) cause {
	if node, ok := e.nodeCauseFor(ev, res); ok {
		return cause{kind: causeNodeCondition, node: node}
	}
	if massKey, ok := e.coveringMassFailure(ev, key, res); ok {
		return cause{kind: causeSharedDependency, mass: massKey}
	}
	if inc, ok := e.ownerIncidentFor(ev, owner, res); ok {
		return cause{kind: causeOwnerWorkload, owner: inc}
	}
	return cause{}
}

// nodeCauseFor reports whether an active node-level incident explains a pod
// event, and which node incident to credit. Pods bound to an inhibited node
// are symptoms of it; unschedulable pods (no node yet) are attributed to the
// most constrained node when any node is down. Caller must hold e.mu.
func (e *Engine) nodeCauseFor(
	ev event.Event,
	res string,
) (*model.Incident, bool) {
	if !e.config.InhibitNodeSuppressesPods || res != "pod" {
		return nil, false
	}
	if ev.NodeName != "" {
		if !e.activeNodeIncidents[ev.NodeName] {
			return nil, false
		}
		return e.findNodeIncident(ev.NodeName), true
	}
	if len(e.activeNodeIncidents) == 0 {
		return nil, false
	}
	return e.findMostConstrainedNodeIncident(), true
}

// coveringMassFailure reports the mass-failure incident, if any, that already
// speaks for an event: one whose shared dependency this event's resource
// depends on. Node events are never covered — a node is the root cause, not a
// symptom of itself. An incident that was announced before the mass failure
// was detected is not covered either: hiding it now would orphan its thread.
// Caller must hold e.mu.
func (e *Engine) coveringMassFailure(
	ev event.Event,
	key model.IncidentKey,
	res string,
) (model.IncidentKey, bool) {
	if res == "node" || len(e.massFailures) == 0 ||
		e.config.DependenciesOf == nil {
		return "", false
	}
	if inc, exists := e.state[key]; exists && inc.SuppressedBy == "" {
		return "", false
	}
	probe := &model.Incident{
		Subject: model.Subject{
			Resource:  res,
			Namespace: ev.Namespace,
			Name:      ev.PodName,
			NodeName:  ev.NodeName,
		},
	}

	for _, dep := range e.config.DependenciesOf(probe) {
		if massKey := MassFailureKey(dep); e.hasMassFailureLocked(massKey) {
			return massKey, true
		}
	}
	return "", false
}

// ownerIncidentFor finds an active non-pod incident on the pod's owning
// workload. Workload incidents may carry their owner as either a bare name
// (pod path) or "ns/name" (workload-object detectors), so both encodings are
// accepted — otherwise a stuck Deployment would never claim its own pods.
// Caller must hold e.mu.
func (e *Engine) ownerIncidentFor(
	ev event.Event,
	owner, res string,
) (*model.Incident, bool) {
	if res != "pod" || owner == "" {
		return nil, false
	}
	ownerFull := OwnerPath(ev.Namespace, owner)
	// Only incidents in the event's own namespace can claim it, so walk the
	// namespace index rather than every incident in the cluster: this runs on
	// every event.
	keys := make([]string, 0, len(e.namespaceIndex[ev.Namespace]))
	for key := range e.namespaceIndex[ev.Namespace] {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		existing := e.namespaceIndex[ev.Namespace][model.IncidentKey(rawKey)]
		if existing.State == model.StateResolved ||
			existing.State == model.StatePendingResolve {
			continue
		}
		if existing.Resource == "pod" {
			continue
		}
		// The owner must match in the incident's Name field and in the owner
		// slot of its key — the key check guards against two distinct owners
		// sharing one namespace prefix.
		if existing.Name != owner && existing.Name != ownerFull {
			continue
		}
		if pk := ParseKey(
			existing.Key,
		); pk.Owner != owner &&
			pk.Owner != ownerFull {
			continue
		}
		return existing, true
	}
	return nil, false
}

// recordSymptom does the bookkeeping for an attributed event and returns what
// Process reports: the incident it touched (if any) and ActionSkip, because a
// symptom is never announced on its own. Caller must hold e.mu.
func (e *Engine) recordSymptom(
	c cause,
	ev event.Event,
	owner string,
	cs *model.ContainerState,
	key model.IncidentKey,
	res string,
	now time.Time,
) (*model.Incident, model.IncidentAction) {
	e.auditSkipOnce(key, ev, c.kind.auditReason())
	switch c.kind {
	case causeNodeCondition:
		if c.node != nil {
			recordSuppressedPod(c.node, ev, owner)
		}
		return nil, model.ActionSkip

	case causeSharedDependency:
		// The incident is created (or refreshed) rather than dropped: it
		// must count toward the mass failure so the blast-radius alert stays
		// open while its symptoms persist, and it must still exist when the
		// mass failure clears so a symptom that outlives its cause can be
		// announced then instead of vanishing.
		inc, exists := e.state[key]
		if !exists {
			inc = e.newIncident(ev, owner, cs, key, res, now)
			e.state[key] = inc
			e.indexIncidentByNamespace(inc)
		} else {
			inc.State = model.StateActive
			inc.ResolveAt = time.Time{}
			addResource(inc, ev.PodName)
			inc.LastContainerState = cs
			inc.Count++
			inc.LastSeen = now
			inc.LastUpdate = now
		}
		e.rememberPodResource(key, ev)
		inc.SuppressedBy = c.mass
		return inc, model.ActionSkip

	case causeOwnerWorkload:
		c.owner.Count++
		addResource(c.owner, ev.PodName)
		e.rememberPodResource(c.owner.Key, ev)
		c.owner.LastSeen = now
		return nil, model.ActionSkip
	}
	return nil, model.ActionSkip
}

// addResource records a pod against an incident and keeps the peak count.
func addResource(inc *model.Incident, podName string) {
	if podName == "" {
		return
	}
	inc.Resources[podName] = true
	if len(inc.Resources) > inc.PeakResources {
		inc.PeakResources = len(inc.Resources)
	}
}

// ReleaseSuppressed clears the suppression the given mass failure imposed and
// announces every incident that is still active: whatever the mass failure
// was speaking for must speak for itself from here on. Returns how many were
// announced. Callers must not hold e.mu.
func (e *Engine) ReleaseSuppressed(massKey model.IncidentKey) int {
	e.mu.Lock()
	released := e.releaseSuppressedLocked(massKey)
	e.mu.Unlock()
	e.emit(released...)
	return len(released)
}

// releaseSuppressedLocked is the decision half of ReleaseSuppressed. Caller
// must hold e.mu and emit the returned transitions after unlocking.
func (e *Engine) releaseSuppressedLocked(
	massKey model.IncidentKey,
) []transition {
	var released []transition
	keys := make([]string, 0, len(e.state))
	for key := range e.state {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		inc := e.state[model.IncidentKey(rawKey)]
		if inc.SuppressedBy != massKey {
			continue
		}
		inc.SuppressedBy = ""
		if inc.State != model.StateActive {
			continue
		}
		if a := e.edgeAction(inc); a != model.ActionSkip {
			released = append(released, transition{inc.Clone(), a})
		}
	}
	e.dirty = true
	return released
}
