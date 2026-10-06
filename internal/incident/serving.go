package incident

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An exposed Service is one that users outside the cluster reach: an
// Ingress or Gateway route sends traffic to it, or it is a LoadBalancer
// or NodePort Service. A workload one of them selects is user-facing.
// A workload only other workloads call, behind a ClusterIP Service, is
// internal: when it fails, the workloads that need it show the user
// impact, and they are the ones judged.

// exposed reports whether users reach the Service from outside.
func exposed(model inventory.Reader, service inventory.EntityID) bool {
	if routed(model, service) {
		return true
	}
	entity, ok := model.Entity(service)
	if !ok {
		return false
	}
	attribute, _ := entity.Attribute(kube.AttrServiceType)
	kind := attribute.Value.AsText()
	return kind == "LoadBalancer" || kind == "NodePort"
}

// servingDown reports an incident whose root or impact holds a
// user-facing workload with none of its replicas ready: its last
// replica is down, so the users of the Service it backs get nothing.
func servingDown(model inventory.Reader, p *Incident) bool {
	for _, id := range append([]inventory.EntityID{p.Root}, p.Impact...) {
		if !IsWorkload(id.Kind) {
			continue
		}
		ready, ok := ReadinessOf(model, id)
		if !ok || ready.Desired == 0 || ready.Ready > 0 {
			continue
		}
		if userFacing(model, id) {
			return true
		}
	}
	return false
}

// userFacing reports a workload that an exposed Service selects.
func userFacing(model inventory.Reader, id inventory.EntityID) bool {
	entity, ok := model.Entity(id)
	if !ok {
		return false
	}
	for _, service := range kube.ServicesSelecting(model, entity) {
		if exposed(model, service) {
			return true
		}
	}
	return false
}
