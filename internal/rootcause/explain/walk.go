package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Walk limits. Real causes sit a few links away; the limits keep one
// solve proportional to the failure, not to the cluster.
const (
	// MaxDepth is the longest chain from a candidate to its effect:
	// zone → node → pod → container is three, a registry behind a
	// rollout is at most five.
	MaxDepth = 6
	// MaxFanIn bounds the links followed out of one entity and the
	// dependents counted for one candidate. Larger sets are sampled
	// deterministically.
	MaxFanIn = 32
)

// hop is one link from an entity to something it depends on.
type hop struct {
	link LinkType
	to   inventory.EntityID
}

// reach is one entity found upstream of an effect.
type reach struct {
	id inventory.EntityID
	// link is how this entity reaches the next one toward the effect.
	link LinkType
	// chain runs from this entity to the effect.
	chain []inventory.EntityID
}

// storedHops maps stored relations to propagation links. Relations
// point from the dependent to its dependency, so they lead upstream.
var storedHops = []struct {
	relation inventory.RelationType
	link     LinkType
}{
	{inventory.OwnedBy, LinkOwns},
	{inventory.RunsOn, LinkRunsOn},
	{inventory.References, LinkUses},
	{inventory.Mounts, LinkMounts},
	{inventory.Serves, LinkServedBy},
	{inventory.ResolvesVia, LinkResolvesVia},
	{inventory.PartOf, LinkContains},
	{inventory.RoutesTo, LinkRoutesTo},
}

// walkItem is one entity on the walk's frontier.
type walkItem struct {
	id    inventory.EntityID
	chain []inventory.EntityID
	depth int
}

// walk returns every entity upstream of effect within MaxDepth, each
// once, by its shortest chain. A container walks from itself and from
// its pod at once: the pod is the same unit of failure, not a cause of
// its own containers.
func (v *view) walk(effect inventory.EntityID) []reach {
	seen := map[inventory.EntityID]bool{effect: true}
	queue := []walkItem{{id: effect, chain: []inventory.EntityID{effect}}}
	if effect.Kind == kube.KindContainer {
		if pod, ok := v.podOf(effect); ok {
			seen[pod] = true
			queue = append(queue, walkItem{id: pod,
				chain: []inventory.EntityID{pod, effect}})
		}
	}
	var out []reach
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= MaxDepth {
			continue
		}
		for _, h := range v.upstream(current.id) {
			if seen[h.to] {
				continue
			}
			seen[h.to] = true
			chain := append([]inventory.EntityID{h.to}, current.chain...)
			out = append(out, reach{id: h.to, link: h.link, chain: chain})
			queue = append(queue, walkItem{id: h.to, chain: chain,
				depth: current.depth + 1})
		}
	}
	return out
}

// upstream lists the hops out of id: stored relations, read-time links
// and virtual dependencies. The result is sorted and sampled.
func (v *view) upstream(id inventory.EntityID) []hop {
	if cached, ok := v.hops[id]; ok {
		return cached
	}
	var out []hop
	for _, stored := range storedHops {
		for _, to := range v.s.Model.Related(
			id, stored.relation, inventory.Outgoing,
		) {
			out = append(out, hop{link: stored.link, to: to})
		}
	}
	out = append(out, v.pullHops(id)...)
	out = append(out, v.managerHops(id)...)
	out = append(out, v.namespaceHop(id)...)
	out = append(out, v.virtualHops(id)...)
	out = sampleHops(uniqueHops(out))
	v.hops[id] = out
	return out
}

// pullHops leads from what pulls an image to the image's registry.
func (v *view) pullHops(id inventory.EntityID) []hop {
	var out []hop
	for _, image := range v.s.Model.Related(
		id, inventory.Pulls, inventory.Outgoing,
	) {
		out = append(out, hop{link: LinkPulls, to: registryOf(image)})
	}
	return out
}

// managerHops follows managed-by, which only read-time links resolve.
func (v *view) managerHops(id inventory.EntityID) []hop {
	if v.s.Links == nil {
		return nil
	}
	var out []hop
	for _, link := range v.s.Links.Links(id) {
		if link.Type == inventory.ManagedBy {
			out = append(out, hop{link: LinkManages, to: link.To})
		}
	}
	return out
}

// namespaceHop leads from a namespaced object to its namespace.
func (v *view) namespaceHop(id inventory.EntityID) []hop {
	if id.Namespace == "" || id.Kind == kube.KindNamespace {
		return nil
	}
	return []hop{{link: LinkContains,
		to: inventory.CoreID(kube.KindNamespace, "", id.Namespace)}}
}

// uniqueHops drops repeated hops, keeping the first link to a target.
func uniqueHops(hops []hop) []hop {
	seen := map[inventory.EntityID]bool{}
	out := hops[:0]
	for _, h := range hops {
		if !seen[h.to] {
			seen[h.to] = true
			out = append(out, h)
		}
	}
	return out
}

// sampleHops sorts hops and keeps at most MaxFanIn, spread evenly over
// the sorted list so the sample is the same on every run.
func sampleHops(hops []hop) []hop {
	sort.Slice(hops, func(i, j int) bool {
		if hops[i].link != hops[j].link {
			return hops[i].link < hops[j].link
		}
		return hops[i].to.String() < hops[j].to.String()
	})
	if len(hops) <= MaxFanIn {
		return hops
	}
	out := make([]hop, 0, MaxFanIn)
	for i := 0; i < MaxFanIn; i++ {
		out = append(out, hops[i*len(hops)/MaxFanIn])
	}
	return out
}

// sampleIDs keeps at most MaxFanIn ids, sorted and evenly spread.
func sampleIDs(ids []inventory.EntityID) []inventory.EntityID {
	sorted := append([]inventory.EntityID(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].String() < sorted[j].String()
	})
	if len(sorted) <= MaxFanIn {
		return sorted
	}
	out := make([]inventory.EntityID, 0, MaxFanIn)
	for i := 0; i < MaxFanIn; i++ {
		out = append(out, sorted[i*len(sorted)/MaxFanIn])
	}
	return out
}
