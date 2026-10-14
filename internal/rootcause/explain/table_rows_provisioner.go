package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// unboundModes are the modes of a claim that cannot get a volume.
var unboundModes = []detection.Mode{detection.ModeClaimFailed,
	detection.ModeVolumeProvisioningFailed, detection.ModePending}

// provisionerHops lead from a claim that cannot get a volume to the
// workloads that serve its StorageClass's provisioner.
func (v *view) provisionerHops(claim inventory.EntityID) []hop {
	if claim.Kind != kube.KindPVC ||
		!v.hasMode(claim, unboundModes) && !v.claimPending(claim) {
		return nil
	}
	name := v.claimProvisioner(claim)
	if name == "" {
		return nil
	}
	return hopsTo(LinkProvisions, v.blameable(
		kube.ProvisionerWorkloads(v.s.Model, name))...)
}

// blameable keeps the serving workloads only when blaming is safe: one
// workload serves the provisioner, or every one that may does and none
// is ready. With several candidates and some ready, kwatch cannot tell
// which one the claim waits for.
func (v *view) blameable(
	workloads []inventory.EntityID,
) []inventory.EntityID {
	if len(workloads) > 1 {
		for _, id := range workloads {
			if !v.noReadyPods(id) {
				return nil
			}
		}
	}
	return workloads
}

// claimPending reports a claim the model shows as Pending: the pod that
// waits for it may fail before the claim has a finding of its own.
func (v *view) claimPending(claim inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(claim)
	return ok && attrText(e, kube.AttrPhase) == "Pending"
}

// claimProvisioner is the provisioner of the claim's StorageClass.
func (v *view) claimProvisioner(claim inventory.EntityID) string {
	for _, id := range v.s.Model.Related(
		claim, inventory.References, inventory.Outgoing,
	) {
		if id.Kind != kube.KindStorageClass {
			continue
		}
		if class, ok := v.s.Model.Entity(id); ok {
			return attrText(class, kube.AttrProvisioner)
		}
	}
	return ""
}

// provisionerModes is ModeProvisionerDown for a workload that serves the
// provisioner of the effect claim's class and has no ready pod.
func (v *view) provisionerModes(id inventory.EntityID, link LinkType,
) []modeHealth {
	if link == LinkProvisions && v.noReadyPods(id) {
		return pseudoFailing(ModeProvisionerDown)
	}
	return nil
}

// noReadyPods reports a workload with no ready replica.
func (v *view) noReadyPods(id inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return false
	}
	if ready, known := attrNumber(e, kube.AttrReadyReplicas); known {
		return ready == 0
	}
	pods := rootcause.OwnedPods(v.s.Model, id)
	for _, pod := range pods {
		if !v.notReady(pod) {
			return false
		}
	}
	return len(pods) > 0
}
