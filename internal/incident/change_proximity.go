package incident

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// How close a changed object sits to the failing root, nearest first.
// A change to the root itself is the most likely cause; one anywhere else
// in the namespace is the least.
const (
	// nearSameObject is the root, the controllers above it, or a pod
	// under it.
	nearSameObject = iota
	// nearConfig is an object the root or its pods use: a ConfigMap, a
	// Secret, a claim, a service account.
	nearConfig
	// nearTraffic is what routes to the root or scales or guards it: its
	// Service, Ingress or route, autoscaler, budget or network policy.
	nearTraffic
	// nearNode is a node one of its pods runs on.
	nearNode
	// nearNamespace is anything else in the root's namespace.
	nearNamespace
	// farAway is outside the namespace and unrelated.
	farAway
)

// maxProximityPods bounds the pods examined for one root, so a
// Deployment of thousands of replicas costs no more than a small one.
const maxProximityPods = 16

// proximity measures graph distance from one root. It is built once per
// message and asked about each changed object.
type proximity struct {
	model     inventory.Reader
	root      inventory.EntityID
	anchors   map[inventory.EntityID]bool
	targets   map[inventory.EntityID]bool
	nodes     map[inventory.EntityID]bool
	usedByAny map[inventory.EntityID]bool
}

func newProximity(
	model inventory.Reader, root inventory.EntityID,
) *proximity {
	p := &proximity{model: model, root: root,
		anchors:   map[inventory.EntityID]bool{root: true},
		targets:   map[inventory.EntityID]bool{root: true},
		nodes:     map[inventory.EntityID]bool{},
		usedByAny: map[inventory.EntityID]bool{}}
	base := root
	if pod, ok := rootcause.PodOf(model, root); ok {
		base = pod
		p.anchors[pod] = true
	}
	for _, owner := range rootcause.OwnerChain(model, base) {
		p.anchors[owner] = true
	}
	top := rootcause.TopOwner(model, base)
	pods := rootcause.OwnedPods(model, top)
	if len(pods) > maxProximityPods {
		pods = pods[:maxProximityPods]
	}
	for id := range p.anchors {
		p.targets[id] = true
	}
	for _, pod := range pods {
		p.targets[pod] = true
		for _, node := range model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			p.nodes[node] = true
		}
	}
	p.collectUses()
	return p
}

// collectUses records what the root, its owners and its pods use.
func (p *proximity) collectUses() {
	for id := range p.targets {
		for _, relation := range []inventory.RelationType{
			inventory.References, inventory.Mounts,
		} {
			for _, used := range p.model.Related(id, relation,
				inventory.Outgoing) {
				p.usedByAny[used] = true
			}
		}
	}
}

// rank is how near id is to the root.
func (p *proximity) rank(id inventory.EntityID) int {
	switch {
	case p.anchors[id]:
		return nearSameObject
	case p.usedByAny[id]:
		return nearConfig
	case p.nodes[id]:
		return nearNode
	case p.fronts(id, 0):
		return nearTraffic
	case id.Namespace == p.root.Namespace:
		return nearNamespace
	}
	return farAway
}

// fronts reports an object that selects, routes to, scales or limits
// the root's pods, directly or through one Service.
func (p *proximity) fronts(id inventory.EntityID, depth int) bool {
	for _, relation := range []inventory.RelationType{
		inventory.Selects, inventory.Scales, inventory.RoutesTo,
	} {
		for _, target := range p.model.Related(id, relation,
			inventory.Outgoing) {
			if p.targets[target] ||
				(depth == 0 && target.Kind == kube.KindService &&
					p.fronts(target, depth+1)) {
				return true
			}
		}
	}
	return false
}
