package kube

import (
	"strings"
	"time"

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
// It is sent on every scan, with no target when the holder's pod is
// unknown: the edge to a former holder must not outlive the hand-over.
func (p *Prober) holderRelation(
	lease inventory.EntityID, holder string, now time.Time,
) inventory.Observation {
	out := inventory.Observation{
		Kind: inventory.Related, Source: ProbeSource, At: now,
		Entity: lease, Relation: inventory.References,
	}
	pod := inventory.CoreID(KindPod, lease.Namespace, HolderPodName(holder))
	if p.cfg.Model != nil && p.cfg.Model.Exists(pod) {
		out.Targets = []inventory.EntityID{pod}
	}
	return out
}
