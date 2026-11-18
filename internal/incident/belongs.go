package incident

import (
	"maps"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Belongs maps an object of an incident to the workload it is part of
// by the cluster's own relations: a pod or container to the controller
// that owns it, an autoscaler to the workload it scales, a Service or
// disruption budget to the one workload whose pods it selects. Writers
// read it to tell a workload's own objects from others that merely
// share its name. An object with no such workload has no entry.
type Belongs map[inventory.EntityID]inventory.EntityID

// Of reports whether id is subject or part of it. known is false when
// the incident holds no relation for id (it is not one of its objects,
// or the incident came from an older record): the caller then decides
// without one.
func (b Belongs) Of(subject, id inventory.EntityID) (owned, known bool) {
	if id == subject {
		return true, true
	}
	w, ok := b[id]
	if !ok {
		return false, false
	}
	return w == subject, true
}

// belongsOf is the workload of every object p names: its root and
// subject, members, impact, merged roots and the cause's chain.
func belongsOf(model inventory.Reader, p *Incident) Belongs {
	ids := []inventory.EntityID{p.Root, p.Subject}
	ids = append(ids, p.Impact...)
	ids = append(ids, p.Merged...)
	for key := range p.Members {
		ids = append(ids, key.Entity)
	}
	if p.Cause != nil {
		ids = append(ids, p.Cause.Root)
		ids = append(ids, p.Cause.Chain...)
	}
	out := Belongs{}
	for _, id := range ids {
		if _, done := out[id]; done || id.Name == "" {
			continue
		}
		if w := workloadOf(model, id); w.Name != "" && w != id {
			out[id] = w
		}
	}
	return out
}

// workloadOf is the workload id is part of, or the zero ID.
func workloadOf(
	model inventory.Reader, id inventory.EntityID,
) inventory.EntityID {
	switch id.Kind {
	case kube.KindHPA:
		for _, target := range model.Related(id, inventory.Scales,
			inventory.Outgoing) {
			return rootcause.TopOwner(model, target)
		}
		return inventory.EntityID{}
	case kube.KindService, kube.KindPDB:
		return selectedWorkload(model, id)
	}
	if pod, ok := rootcause.PodOf(model, id); ok {
		id = pod
	}
	return rootcause.TopOwner(model, id)
}

// selectedWorkload is the one workload whose pods id selects, or the
// zero ID when it selects none or the pods of several.
func selectedWorkload(
	model inventory.Reader, id inventory.EntityID,
) inventory.EntityID {
	var only inventory.EntityID
	for _, link := range kube.Links(model, id) {
		if link.Type != inventory.Selects {
			continue
		}
		w := rootcause.TopOwner(model, link.To)
		if w == link.To || only.Name != "" && w != only {
			return inventory.EntityID{}
		}
		only = w
	}
	return only
}

// clone is a copy safe to hand to writers.
func (b Belongs) clone() Belongs {
	return maps.Clone(b)
}
