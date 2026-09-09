package correlation

import (
	"github.com/abahmed/kwatch/internal/model"
)

// Resolve records an observed recovery: on this subject, this reason is no
// longer true. An empty reason resolves every reason the subject has open,
// which is what a detector that finds nothing wrong is saying.
//
// The subject is a typed reference rather than a spelled-out incident key.
// Six packages used to build keys themselves -- resolving BuildKey(ns, owner,
// reason, "") -- which meant the key format, including which slot a container
// occupies and what an empty namespace means, was duplicated everywhere a
// recovery could be reported. Matching by reference also means a
// caller no longer has to know that a pod incident is keyed by its owning
// workload while a Deployment incident is keyed by "namespace/name".
//
// Group, mass-failure and cross-namespace global incidents are deliberately
// left alone: they speak for many subjects at once, so one subject recovering
// does not answer for them. They close through their own paths.
func (e *Engine) Resolve(subject model.ObjectRef, reason string) {
	want := model.NewObjectRef(
		subject.Kind, subject.Namespace, subject.Name,
	)
	if want.Name == "" {
		return
	}
	var pending []transition
	baselineCleared := false
	changed := false

	e.mu.Lock()
	if e.frozen {
		e.mu.Unlock()
		return
	}
	now := e.now()
	// An observed recovery retires the owner-level baseline for this subject
	// whether or not an incident is open for it.
	if e.clearOwnerBaselineFor(want) {
		baselineCleared = true
		changed = true
	}
	// The subject index is what makes this proportional to the incidents
	// about this one object. Walking every incident in the cluster here cost
	// a full locked scan per healthy object per resync, which on a large
	// cluster is most of what the engine did.
	for _, key := range e.incidentsForSubject(want) {
		inc := e.state[key]
		if inc == nil || !subjectScoped(key) {
			continue
		}
		if reason != "" &&
			normalizeReason(inc.Reason) != normalizeReason(reason) {
			continue
		}
		if inc.State == model.StateResolved ||
			inc.State == model.StatePendingResolve {
			continue
		}
		// For pod incidents owned by a workload, gate on workload health.
		if !e.isOwnerHealthy(inc) {
			continue
		}
		if e.config.ResolveHoldDown > 0 {
			e.holdDownLocked(inc, now)
			changed = true
			continue
		}
		changed = true
		pending = append(pending, e.resolveLocked(key, inc, now))
	}
	// The dirty flag is set only when something actually moved. Most calls
	// are a healthy object reporting in on a resync and change nothing;
	// marking the engine dirty for those made every snapshot look like a
	// change and kept rewriting the state ConfigMap on a healthy cluster.
	if changed {
		e.dirty = true
	}
	e.mu.Unlock()

	e.emit(pending...)
	if baselineCleared || len(pending) > 0 {
		e.publishBaseline()
	}
}

// ResolveObserved resolves exactly what the given observation would have
// reported. A producer describes "this is broken" and "this recovered" with
// the same observation, so the two can never disagree about which incident
// they mean. Use it when the producer holds the observation; use Resolve when
// it holds only the object.
func (e *Engine) ResolveObserved(obs *model.Observation) {
	if obs == nil {
		return
	}
	e.markResolved(ObservationKey(obs))
}

// subjectScoped reports whether a key belongs to exactly one Kubernetes
// object, which is the only kind of incident one subject's recovery can
// answer for.
func subjectScoped(key model.IncidentKey) bool {
	return !IsGroupKey(key) && !IsMassFailureKey(key) && !IsGlobalKey(key)
}

// clearOwnerBaselineFor retires the owner-level baseline entries recorded for
// a subject, in whichever spelling its incidents are keyed by: a pod incident
// is keyed by the bare owner name, an object incident by "namespace/name".
// Caller must hold e.mu.
func (e *Engine) clearOwnerBaselineFor(subject model.ObjectRef) bool {
	cleared := e.clearOwnerBaseline(subject.Namespace, subject.Name)
	if subject.Namespace != "" {
		if e.clearOwnerBaseline(
			subject.Namespace, subject.Namespace+"/"+subject.Name,
		) {
			cleared = true
		}
	}
	return cleared
}
