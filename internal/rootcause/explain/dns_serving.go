package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// RowDNSServing names the row that blames the workload serving the
// cluster DNS for lookups that fail while its pods run and are ready.
const RowDNSServing = "dns-servers-not-resolving"

// LinkResolvesWith links a pod whose lookups fail to the workload that
// serves the cluster DNS.
const LinkResolvesWith LinkType = "resolves-with"

// dnsServingRows blame the DNS server workload when lookups fail and its
// servers all run: the servers answer, but not for what the clients ask
// (an upstream resolver that stops answering, a pod replaced under them).
// A down server or a failing probe is the cluster DNS outage instead.
var dnsServingRows = []Row{{
	Name: RowDNSServing,
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeResolution}},
	Link: LinkResolvesWith,
	Effect: Side{Kind: kube.KindPod, Modes: podFailures,
		Signal: SignalDNS},
	Prior: 0.7, InferredMinWorkloads: SharedMinWorkloads,
}}

// dnsServingHops lead from a pod whose lookups fail to the workload that
// serves the cluster DNS, when its servers are all ready.
func (v *view) dnsServingHops(pod inventory.EntityID) []hop {
	if !v.dnsServersReady() || !v.unitDNS(pod) {
		return nil
	}
	var out []hop
	seen := map[inventory.EntityID]bool{}
	for _, server := range kube.DNSServerPods(v.s.Model) {
		top := rootcause.TopOwner(v.s.Model, server)
		if top != server && !seen[top] {
			seen[top] = true
			out = append(out, hop{link: LinkResolvesWith, to: top})
		}
	}
	return out
}

// dnsServersReady reports cluster DNS servers that all run ready, with
// the cluster DNS itself neither unavailable nor failing its probe.
func (v *view) dnsServersReady() bool {
	servers := kube.DNSServerPods(v.s.Model)
	if len(servers) == 0 || v.failing(kube.ClusterDNS) {
		return false
	}
	for _, server := range servers {
		e, ok := v.s.Model.Entity(server)
		if !ok || !attrBool(e, kube.AttrReady) || v.unitFailing(server) {
			return false
		}
	}
	return true
}

// dnsServingModes is the pseudo mode of the DNS server workload for a
// pod whose lookups fail.
func (v *view) dnsServingModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link == LinkResolvesWith && kube.IsDNSWorkload(v.s.Model, id) &&
		v.unitDNS(effect) {
		return pseudoFailing(ModeResolution)
	}
	return nil
}

// unitDNS reports a pod or container whose own error, or its pod's or
// containers', shows name resolution failing.
func (v *view) unitDNS(id inventory.EntityID) bool {
	if hasSignal(SignalDNS, v.text(id)) {
		return true
	}
	if id.Kind != kube.KindPod {
		return false
	}
	return anyMember(v.s.Model.Related(id, inventory.PartOf,
		inventory.Incoming), func(c inventory.EntityID) bool {
		return hasSignal(SignalDNS, v.text(c))
	})
}
