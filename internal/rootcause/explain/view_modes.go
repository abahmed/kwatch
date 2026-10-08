package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The health modes of each kind of entity a cause can be.

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
	for _, owner := range rootcause.OwnerChain(v.s.Model, id) {
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
	if modes := preemptionModes(link); len(modes) > 0 {
		return modes
	}
	switch id.Kind {
	case kube.KindRegistry:
		if class, ok := v.registryClass(effect); ok {
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
		return v.groupModes(id)
	case kube.KindPVC:
		if modes := v.claimModes(id, effect, link); len(modes) > 0 {
			return modes
		}
	case KindScheduling:
		return failing(ModeRejectsNodes)
	case KindExternalEndpoint, KindFailureSignature:
		return v.calledModes(id, effect, link)
	case kube.KindService:
		if modes := v.backendModes(id, effect, link); len(modes) > 0 {
			return modes
		}
	}
	if modes := v.callModes(id, effect, link); len(modes) > 0 {
		return modes
	}
	return v.missingModes(id, link)
}

// groupModes is ModeMembersFailing for a zone or pool with several
// broken nodes. A booting group has young nodes that fail while they
// come up: that is the boot, not a failing group. One failing node is
// that node's problem, not its group's.
func (v *view) groupModes(group inventory.EntityID) []modeHealth {
	if kube.GroupBootRemaining(v.s.Model, group, v.s.Now) > 0 ||
		v.membersFailing(group) < MinGroupMembersFailing {
		return nil
	}
	return []modeHealth{{mode: ModeMembersFailing,
		health: detection.Failing, pseudo: true}}
}

// backendModes are the pseudo modes of a webhook's backend Service, read
// from the webhook's own finding: the Service does not exist, or it has
// no ready endpoints. A Service scaled to zero has no finding of its own,
// so the webhooks behind it would each be a root; with these modes the
// Service is the one root they share, and the rows that use them ask for
// more than one webhook before inferring it.
func (v *view) backendModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkServedBy || !isAdmissionKind(effect.Kind) {
		return nil
	}
	failing := func(mode detection.Mode) []modeHealth {
		return []modeHealth{{mode: mode, health: detection.Failing,
			pseudo: true}}
	}
	if !v.s.Model.Exists(id) {
		return failing(ModeMissing)
	}
	for _, m := range findingModes(v.s.Findings[effect]) {
		if m.mode == detection.ModeWebhookNoEndpoints {
			return failing(detection.ModeNoEndpoints)
		}
	}
	return nil
}

func isAdmissionKind(kind inventory.Kind) bool {
	for _, k := range admitKinds {
		if k == kind {
			return true
		}
	}
	return false
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
	if modes, ok := v.finalizerModes(id, effect, link); ok {
		return modes, true
	}
	return v.storageModes(id, effect, link)
}

// membersFailing counts the broken nodes of a zone or pool. A node
// under strain, with a CPU stall or high usage, is not a broken node:
// two strained nodes do not make their pool fail as a whole, and a
// pool blamed for them would take over every workload incident on
// those nodes and hand them back when the strain passes.
func (v *view) membersFailing(group inventory.EntityID) int {
	count := 0
	for _, node := range v.s.Model.Related(
		group, inventory.PartOf, inventory.Incoming,
	) {
		if v.broken(node) {
			count++
		}
	}
	return count
}

// broken reports whether id has a failing finding, one that says it
// does not do its job: not ready, under pressure, its network
// unavailable. A degraded finding, such as a CPU stall or high usage,
// does not count.
func (v *view) broken(id inventory.EntityID) bool {
	for _, f := range v.s.Findings[id] {
		if f.Health == detection.Failing {
			return true
		}
	}
	return false
}

// claimModes is the pseudo mode of a claim that pins the effect pod to
// a zone it cannot be placed in; nothing otherwise.
func (v *view) claimModes(
	claim, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkMounts || !v.pinnedClaim(claim, effect) {
		return nil
	}
	return []modeHealth{{mode: ModeVolumePinned, health: detection.Failing,
		pseudo: true}}
}
