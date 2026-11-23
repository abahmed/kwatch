package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkResolvesViaWorkload links a pod whose lookups fail on a resolver
// address to the failing workload that holds that address.
const LinkResolvesViaWorkload LinkType = "resolves-via-workload"

// ModeResolverDown is the pseudo mode of a failing workload that holds
// the resolver address other pods' lookups time out on.
const ModeResolverDown detection.Mode = "ResolverDown"

// RowResolverWorkload names the row below.
const RowResolverWorkload = "resolver-workload-failing"

// resolverWorkloadRows blame a failing workload whose own errors name
// the address that other pods' lookups fail on: a resolver other than
// the cluster DNS, such as a per-node cache, which is not the cluster
// DNS's outage.
var resolverWorkloadRows = []Row{{
	Name: RowResolverWorkload,
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeResolverDown}},
	Link:   LinkResolvesViaWorkload,
	Effect: Side{Kind: kube.KindPod, Modes: podFailures},
	Prior:  0.75,
}}

var resolverAddress = regexp.MustCompile(
	`lookup \S+ on ([0-9][0-9.]+|\[[0-9a-fA-F:]+\]):53\b`)

// resolverOf is the address a pod's lookups failed on.
func (v *view) resolverOf(pod inventory.EntityID) string {
	m := resolverAddress.FindStringSubmatch(v.unitErrorText(pod))
	if m == nil {
		return ""
	}
	return m[1]
}

// resolverHolders are the workloads other than the pod's own that have
// a failing pod whose own error names the resolver address, and which
// are not themselves clients failing lookups on it.
func (v *view) resolverHolders(
	pod inventory.EntityID,
) []inventory.EntityID {
	addr := v.resolverOf(pod)
	if addr == "" {
		return nil
	}
	own := rootcause.TopOwner(v.s.Model, pod)
	seen := map[inventory.EntityID]bool{}
	for id, found := range v.s.Findings {
		if len(found) == 0 || (id.Kind != kube.KindPod &&
			id.Kind != kube.KindContainer) {
			continue
		}
		unit, ok := v.unitOf(id)
		if !ok {
			continue
		}
		top := rootcause.TopOwner(v.s.Model, unit)
		if top == own || top == unit || v.resolverOf(unit) == addr ||
			!strings.Contains(v.unitErrorText(unit), addr+":") {
			continue
		}
		seen[top] = true
	}
	return sortedKeys(seen)
}

// resolverWorkloadHops lead from a pod whose lookups fail to the
// workloads that hold the resolver address.
func (v *view) resolverWorkloadHops(pod inventory.EntityID) []hop {
	var out []hop
	for _, top := range v.resolverHolders(pod) {
		out = append(out, hop{link: LinkResolvesViaWorkload, to: top})
	}
	return out
}

func (v *view) resolverWorkloadModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkResolvesViaWorkload {
		return nil
	}
	if unit, ok := v.unitOf(effect); ok {
		for _, top := range v.resolverHolders(unit) {
			if top == id {
				return pseudoFailing(ModeResolverDown)
			}
		}
	}
	return nil
}

// withoutClusterDNSBehindResolver drops the cluster DNS from the hops of
// a pod whose lookups fail on an address a workload holds: the lookups
// never went to the cluster DNS.
func (v *view) withoutClusterDNSBehindResolver(
	id inventory.EntityID, hops []hop,
) []hop {
	if id.Kind != kube.KindPod || len(v.resolverHolders(id)) == 0 {
		return hops
	}
	out := hops[:0:0]
	for _, h := range hops {
		if h.to != kube.ClusterDNS {
			out = append(out, h)
		}
	}
	return out
}
