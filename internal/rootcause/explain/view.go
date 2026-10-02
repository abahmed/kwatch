package explain

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// view is a Snapshot with the lookups one solve repeats, memoised.
// It lives for one call of Explain.
type view struct {
	s       Snapshot
	since   time.Time
	hops    map[inventory.EntityID][]hop
	selects map[inventory.EntityID]map[inventory.EntityID]bool
	texts   map[inventory.EntityID]string
	changes map[inventory.EntityID][]inventory.Change
	// explainedBy marks the failures some candidate other than their
	// own workload explains. It is filled once candidates are known.
	explainedBy map[inventory.EntityID]bool
	// rejected collects why entities were dropped.
	rejected map[inventory.EntityID]rejectNote
	// inputs lists, per failure, the entities its walk went through.
	// A change to any of them can change the failure's explanation.
	inputs map[inventory.EntityID][]inventory.EntityID
	// gates lists, per entity, the entities whose findings decide
	// which hops lead out of it. They are inputs of every walk through
	// the entity: when one starts failing, the walk can reach further.
	gates map[inventory.EntityID][]inventory.EntityID
	// dnsSeen memoises dnsSignalled; nil until it is computed.
	dnsSeen *bool
	// rbac memoises changedRBAC; nil until it is computed.
	rbac *[]inventory.EntityID
	// removed memoises removedNodes; nil until it is computed.
	removed *[]inventory.EntityID
	// unverified names, per failure, the kinds kwatch could not see
	// where a cause might have been.
	unverified map[inventory.EntityID][]string
	// calls and signatures memoise callOf and signatureOf per pod; an
	// empty value means the pod names none.
	calls      map[inventory.EntityID]endpointCall
	signatures map[inventory.EntityID]string
}

func newView(s Snapshot) *view {
	return &view{
		s: s, since: s.Now.Add(-s.window()),
		hops:       map[inventory.EntityID][]hop{},
		selects:    map[inventory.EntityID]map[inventory.EntityID]bool{},
		texts:      map[inventory.EntityID]string{},
		changes:    map[inventory.EntityID][]inventory.Change{},
		inputs:     map[inventory.EntityID][]inventory.EntityID{},
		gates:      map[inventory.EntityID][]inventory.EntityID{},
		unverified: map[inventory.EntityID][]string{},
		calls:      map[inventory.EntityID]endpointCall{},
		signatures: map[inventory.EntityID]string{},
	}
}

// noteInputs records what the walk from a failure went through,
// including the gates of every entity it expanded.
func (v *view) noteInputs(effect inventory.EntityID, reached []reach) {
	ids := []inventory.EntityID{effect}
	if pod, ok := v.podOf(effect); ok && pod != effect {
		ids = append(ids, pod)
	}
	for _, r := range reached {
		ids = append(ids, r.id)
	}
	walked := ids
	for _, id := range walked {
		ids = append(ids, v.gates[id]...)
	}
	v.inputs[effect] = ids
}

// gate records that whether a hop leads out of from depends on the
// findings of to.
func (v *view) gate(from, to inventory.EntityID) {
	for _, known := range v.gates[from] {
		if known == to {
			return
		}
	}
	v.gates[from] = append(v.gates[from], to)
}

// failing reports whether id has a Failing or Degraded finding.
func (v *view) failing(id inventory.EntityID) bool {
	for _, f := range v.s.Findings[id] {
		if unhealthy(f) {
			return true
		}
	}
	return false
}

// unitFailing reports whether a pod or any of its containers fails.
func (v *view) unitFailing(id inventory.EntityID) bool {
	if v.failing(id) {
		return true
	}
	if id.Kind != kube.KindPod {
		return false
	}
	for _, container := range v.s.Model.Related(
		id, inventory.PartOf, inventory.Incoming,
	) {
		if v.failing(container) {
			return true
		}
	}
	return false
}

// unitGate is unitFailing for a hop out of a pod: the pod's containers
// gate the hop.
func (v *view) unitGate(pod inventory.EntityID) bool {
	for _, container := range v.s.Model.Related(
		pod, inventory.PartOf, inventory.Incoming,
	) {
		v.gate(pod, container)
	}
	return v.unitFailing(pod)
}

// changesOf returns id's changes inside the causal window.
func (v *view) changesOf(id inventory.EntityID) []inventory.Change {
	if v.s.Changes == nil {
		return nil
	}
	if cached, ok := v.changes[id]; ok {
		return cached
	}
	out := dropHarmlessCreations(v.s.Changes.Changes(id, v.since, v.s.Now))
	v.changes[id] = out
	return out
}

// dropHarmlessCreations removes the creation of a ConfigMap, Secret or
// ServiceAccount. Creating one can only satisfy a reference, never break
// it, and a new namespace creates kube-root-ca.crt and the default
// ServiceAccount that every pod in it references: counting them would
// blame a healthy bootstrap object for the pod's own failure.
func dropHarmlessCreations(changes []inventory.Change) []inventory.Change {
	out := changes[:0:0]
	for _, change := range changes {
		if change.Created && harmlessToCreate(change.Entity.Kind) {
			continue
		}
		out = append(out, change)
	}
	return out
}

func harmlessToCreate(kind inventory.Kind) bool {
	switch kind {
	case kube.KindConfigMap, kube.KindSecret, kube.KindAccount:
		return true
	}
	return false
}

// text is an effect's error text: its findings and the recent notes of
// the entity and its pod. Signals and specificity read it.
func (v *view) text(id inventory.EntityID) string {
	if cached, ok := v.texts[id]; ok {
		return cached
	}
	var parts []string
	for _, f := range v.s.Findings[id] {
		parts = append(parts, f.Summary)
		for _, e := range f.Evidence {
			parts = append(parts, e.Value)
		}
	}
	sources := []inventory.EntityID{id}
	if pod, ok := v.podOf(id); ok && pod != id {
		sources = append(sources, pod)
	}
	for _, source := range sources {
		for _, note := range v.s.Model.Notes(source, v.since) {
			parts = append(parts, note.Message)
		}
	}
	out := strings.Join(parts, " ")
	v.texts[id] = out
	return out
}

// effectState is how a failing entity is matched as a row's effect.
func (v *view) effectState(id inventory.EntityID) state {
	return state{id: id, modes: findingModes(v.s.Findings[id]),
		text: v.text(id)}
}

// causeState is how an upstream entity is matched as a row's cause:
// its findings plus the pseudo modes of changes, absence and virtual
// kinds. The effect matters for virtual kinds, whose state is read from
// the effect's error text.
func (v *view) causeState(
	id, effect inventory.EntityID, link LinkType,
) state {
	if link == LinkAdmits && v.admitsElsewhere(id, effect) {
		return state{id: id}
	}
	modes := findingModes(v.s.Findings[id])
	modes = append(modes, v.changeModes(id)...)
	if link == LinkSelf {
		// A workload's own faults are read from the failing container.
		modes = append(modes, v.selfConfigModes(effect)...)
	} else {
		modes = append(modes, v.virtualModes(id, effect, link)...)
	}
	return state{id: id, modes: modes}
}

// changeModes turns recent changes into pseudo modes. The change of a
// controller whose owner also changed belongs to the owner's rollout
// (a Deployment edit creates a ReplicaSet), so only the owner keeps it.
func (v *view) changeModes(id inventory.EntityID) []modeHealth {
	if v.ownerChanged(id) {
		return nil
	}
	var out []modeHealth
	for _, change := range v.changesOf(id) {
		out = append(out, modeHealth{mode: changeMode(change),
			health: detection.Failing, pseudo: true})
	}
	return out
}

// ownerChanged reports whether an owner of id changed in the window.
func (v *view) ownerChanged(id inventory.EntityID) bool {
	for _, owner := range ownerChain(v.s.Model, id) {
		for _, change := range v.changesOf(owner) {
			if changeMode(change) != ModeScaled {
				return true
			}
		}
	}
	return false
}

// changeMode names a change: a creation, a scale or a spec change.
func changeMode(change inventory.Change) detection.Mode {
	if change.Created {
		return ModeCreated
	}
	if change.Deleted {
		return ModeDeleted
	}
	if len(change.Fields) == 0 {
		return ModeChanged
	}
	for _, field := range change.Fields {
		if field.Path != "spec.replicas" {
			return ModeSpecChanged
		}
	}
	return ModeScaled
}

// virtualModes are the pseudo modes of absent and virtual entities.
func (v *view) virtualModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	failing := func(mode detection.Mode) []modeHealth {
		return []modeHealth{{mode: mode, health: detection.Failing,
			pseudo: true}}
	}
	if modes, ok := v.objectModes(id, effect, link); ok {
		return modes
	}
	switch id.Kind {
	case kube.KindRegistry:
		if class := classifyPull(v.text(effect)); class != pullImage {
			return failing(class)
		}
	case kindClusterDNS:
		if v.resolutionFailing(effect, link) {
			return failing(ModeResolution)
		}
	case kube.KindAPIService:
		if link == LinkServedBy && hasSignal(SignalMetricsAPI,
			v.text(effect)) {
			return failing(ModeMetricsUnserved)
		}
	case kube.KindZone, kube.KindNodePool:
		// One failing node is that node's problem, not its zone's.
		if v.membersFailing(id) >= MinGroupMembersFailing {
			return failing(ModeMembersFailing)
		}
	case KindScheduling:
		return failing(ModeRejectsNodes)
	case KindExternalEndpoint, KindFailureSignature:
		return v.calledModes(id, effect, link)
	}
	return v.missingModes(id, link)
}

// missingModes is ModeMissing for an object named by reference (used,
// mounted or routed to) that does not exist.
func (v *view) missingModes(
	id inventory.EntityID, link LinkType,
) []modeHealth {
	if (link == LinkUses || link == LinkMounts || link == LinkRoutesTo) &&
		!v.s.Model.Exists(id) {
		return []modeHealth{{mode: ModeMissing, health: detection.Failing,
			pseudo: true}}
	}
	return nil
}

// objectModes are the pseudo modes of kinds whose state is read from
// the object together with the effect's error text: admission webhooks
// and the storage chain. ok is false for every other kind.
func (v *view) objectModes(
	id, effect inventory.EntityID, link LinkType,
) (modes []modeHealth, ok bool) {
	switch id.Kind {
	case kube.KindValidatingHook, kube.KindMutatingWebhook:
		return v.admissionModes(id, effect, link), true
	}
	return v.storageModes(id, effect, link)
}

// membersFailing counts the failing nodes of a zone or pool.
func (v *view) membersFailing(group inventory.EntityID) int {
	count := 0
	for _, node := range v.s.Model.Related(
		group, inventory.PartOf, inventory.Incoming,
	) {
		if v.failing(node) {
			count++
		}
	}
	return count
}
