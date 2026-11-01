package kube

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// HolderPodName is the pod a Lease holder identity names. Leader
// election writes "<pod>_<id>"; the pod name is the part before the
// underscore, or the whole identity when there is none.
func HolderPodName(holder string) string {
	name, _, _ := strings.Cut(holder, "_")
	return name
}

// holderRelation links a Lease to the pod that holds it, when that pod
// is known, so a failing holder is found as the cause of a stale Lease.
func (p *Prober) holderRelation(
	lease inventory.EntityID, holder string,
) (inventory.Observation, bool) {
	if p.cfg.Model == nil {
		return inventory.Observation{}, false
	}
	pod := inventory.CoreID(KindPod, lease.Namespace, HolderPodName(holder))
	if !p.cfg.Model.Exists(pod) {
		return inventory.Observation{}, false
	}
	return inventory.Observation{
		Kind: inventory.Related, Source: ProbeSource, At: p.cfg.Now(),
		Entity: lease, Relation: inventory.References,
		Targets: []inventory.EntityID{pod},
	}, true
}
