package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leadSubject is the entity the lead sentence is about. It follows the
// choice leadText makes.
func leadSubject(f caseFacts) inventory.EntityID {
	p := f.p
	switch {
	case ownCause(p.Cause), blamedChange(f) != nil:
		return affectedWorkload(f)
	case p.Cause != nil && rootFinding(p, f.members) == nil:
		return symptomSubject(f)
	}
	return p.Root
}

// affectedWorkload is the workload a cause broke: the first workload in
// the cause chain, else the workload running the failing pod, else the
// root.
func affectedWorkload(f caseFacts) inventory.EntityID {
	if f.p.Cause != nil {
		for _, id := range f.p.Cause.Chain {
			if isWorkload(id.Kind) {
				return id
			}
		}
	}
	if f.ok {
		if owner, ok := ownerIn(f.p.Impact, f.lead.Entity); ok {
			return owner
		}
	}
	return f.p.Root
}

// symptomSubject is what the reader runs: the workload hit by the
// cause, else the failing entity itself.
func symptomSubject(f caseFacts) inventory.EntityID {
	if workload := affectedWorkload(f); isWorkload(workload.Kind) {
		return workload
	}
	if f.ok {
		return f.lead.Entity
	}
	return f.p.Root
}

// ownerIn finds the workload in impact that runs a pod or container.
// Pods are named after their workload ("cart-7d9f-a" belongs to cart),
// so the longest matching workload name wins.
func ownerIn(
	impact []inventory.EntityID, id inventory.EntityID,
) (inventory.EntityID, bool) {
	if id.Kind != kube.KindPod && id.Kind != kube.KindContainer {
		return inventory.EntityID{}, false
	}
	pod, _ := splitContainer(id.Name)
	if id.Kind == kube.KindPod {
		pod = id.Name
	}
	var best inventory.EntityID
	for _, w := range impact {
		if isWorkload(w.Kind) && w.Namespace == id.Namespace &&
			strings.HasPrefix(pod, w.Name+"-") &&
			len(w.Name) > len(best.Name) {
			best = w
		}
	}
	return best, best.Name != ""
}

// leadName names the lead's subject with its namespace and, when one is
// configured, the cluster: "payments in shop (prod-eu-1)".
func (f caseFacts) leadName(id inventory.EntityID) string {
	return placedName(id) + f.clusterTag()
}

// clusterTag is " (prod-eu-1)", or nothing without a cluster name.
func (f caseFacts) clusterTag() string {
	if f.cluster == "" {
		return ""
	}
	return " (" + f.cluster + ")"
}
