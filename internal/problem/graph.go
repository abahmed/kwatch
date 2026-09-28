package problem

import (
	"sort"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/reason"
)

// defaultRoot maps a root to the entity problems are keyed by: a pod or
// container belongs to its workload, so replicas and their controller
// share one problem. Other roots are kept.
func defaultRoot(
	model knowledge.Reader, id knowledge.EntityID,
) knowledge.EntityID {
	if pod, ok := reason.PodOf(model, id); ok {
		return reason.TopOwner(model, pod)
	}
	return id
}

func ownedPods(
	model knowledge.Reader, root knowledge.EntityID,
) []knowledge.EntityID {
	return reason.OwnedPods(model, root)
}

// impact walks downstream from the problem's failing members to what
// users feel: the workloads that own them, and the Services and Ingresses
// routing to them.
func impact(model knowledge.Reader, p *Problem) []knowledge.EntityID {
	seen := map[knowledge.EntityID]bool{p.Root: true}
	var out []knowledge.EntityID
	add := func(id knowledge.EntityID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for key := range p.Members {
		add(key.Entity)
		pod, ok := reason.PodOf(model, key.Entity)
		if !ok {
			continue
		}
		add(reason.TopOwner(model, pod))
		for _, service := range servicesOf(model, pod) {
			add(service)
			for _, ingress := range model.Related(
				service, knowledge.RoutesTo, knowledge.Incoming,
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
	model knowledge.Reader, pod knowledge.EntityID,
) []knowledge.EntityID {
	var out []knowledge.EntityID
	for _, slice := range model.Related(
		pod, knowledge.RoutesTo, knowledge.Incoming,
	) {
		out = append(out, model.Related(
			slice, knowledge.Backs, knowledge.Outgoing)...)
	}
	return out
}

func userFacing(impact []knowledge.EntityID) bool {
	for _, id := range impact {
		if id.Kind == kube.KindIngress {
			return true
		}
	}
	return false
}
