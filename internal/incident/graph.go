package incident

import (
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// defaultRoot maps a root to the entity incidents are keyed by: a pod or
// container belongs to its workload, so replicas and their controller
// share one incident. Other roots are kept.
func defaultRoot(
	model inventory.Reader, id inventory.EntityID,
) inventory.EntityID {
	if pod, ok := rootcause.PodOf(model, id); ok {
		return rootcause.TopOwner(model, pod)
	}
	return id
}

// impact walks downstream from the incident's failing members to what
// users feel: the workloads that own them, and the Services and Ingresses
// routing to them. A member that is not a pod counts as its top owner.
func impact(model inventory.Reader, p *Incident) []inventory.EntityID {
	seen := map[inventory.EntityID]bool{p.Root: true}
	var out []inventory.EntityID
	add := func(id inventory.EntityID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for key := range p.Members {
		pod, ok := rootcause.PodOf(model, key.Entity)
		if !ok {
			// A ReplicaSet and its Deployment are one workload.
			add(rootcause.TopOwner(model, key.Entity))
			continue
		}
		add(key.Entity)
		add(rootcause.TopOwner(model, pod))
		for _, service := range servicesOf(model, pod) {
			add(service)
			for _, ingress := range model.Related(
				service, inventory.RoutesTo, inventory.Incoming,
			) {
				add(ingress)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

func servicesOf(
	model inventory.Reader, pod inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, slice := range model.Related(
		pod, inventory.RoutesTo, inventory.Incoming,
	) {
		out = append(out, model.Related(
			slice, inventory.Backs, inventory.Outgoing)...)
	}
	return out
}

// trafficLost reports whether the root or a Service in the impact that
// users reach (see exposed) has lost its backends: it does not exist,
// none of its backends is ready, or more than half of them are failing.
// A Service without EndpointSlices says nothing, so it does not count.
func trafficLost(model inventory.Reader, p *Incident) bool {
	for _, id := range append([]inventory.EntityID{p.Root}, p.Impact...) {
		if id.Kind != kube.KindService || !exposed(model, id) {
			continue
		}
		if backendsLost(model, id) {
			return true
		}
	}
	return false
}

// routedMissing reports a root Service that does not exist while an
// Ingress or route sends traffic to it.
func routedMissing(model inventory.Reader, p *Incident) bool {
	return p.Root.Kind == kube.KindService && routed(model, p.Root) &&
		!model.Exists(p.Root)
}

// routed reports whether an Ingress or route sends traffic to service.
func routed(model inventory.Reader, service inventory.EntityID) bool {
	for _, from := range model.Related(
		service, inventory.RoutesTo, inventory.Incoming,
	) {
		if hasKind(trafficKinds, from.Kind) {
			return true
		}
	}
	return false
}

// backendsLost sums the Service's EndpointSlices. A Service that does
// not exist has lost everything routed to it.
func backendsLost(model inventory.Reader, service inventory.EntityID) bool {
	if _, ok := model.Entity(service); !ok {
		return true
	}
	slices := model.Related(service, inventory.Backs, inventory.Incoming)
	if len(slices) == 0 {
		return false
	}
	var total, ready float64
	for _, id := range slices {
		slice, ok := model.Entity(id)
		if !ok {
			continue
		}
		total += attrNumber(slice, kube.AttrEndpoints)
		ready += attrNumber(slice, kube.AttrEndpointsReady)
	}
	failing := total - ready
	return ready == 0 || failing*2 > total
}

func attrNumber(e inventory.Entity, name string) float64 {
	a, ok := e.Attribute(name)
	if !ok {
		return 0
	}
	n, _ := a.Value.AsNumber()
	return n
}
