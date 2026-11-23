package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkConfiguresDNS is the link from a failing lookup to an object the
// cluster DNS is configured by: its Service, its configuration or the
// role its servers read the cluster with.
const LinkConfiguresDNS LinkType = "configures-dns"

// dnsConfigRows blame an object of the cluster DNS that changed before
// lookups began to fail, ahead of the DNS as a whole. Each has a row for
// the clients' failures and one for the DNS's own probe.
var dnsConfigRows = concatRows(
	dnsConfigRowPair("dns-config-changed", kube.KindConfigMap,
		[]detection.Mode{ModeChanged}, []detection.Mode{ModeCreated}),
	dnsConfigRowPair("dns-role-changed", "clusterrole",
		[]detection.Mode{ModeChanged}, []detection.Mode{ModeCreated}),
	dnsConfigRowPair("dns-service-changed", kube.KindService,
		[]detection.Mode{ModeChanged, detection.ModeNoEndpoints}, nil),
)

// dnsConfigRowPair is the row for a changed DNS object and the row of
// the same cause whose effect is the cluster DNS itself.
func dnsConfigRowPair(
	name string, kind inventory.Kind, modes, not []detection.Mode,
) []Row {
	cause := Side{Kind: kind, Modes: modes, NotModes: not}
	return []Row{{
		Name: name, Cause: cause, Link: LinkConfiguresDNS,
		Effect: Side{Kind: kube.KindPod, Modes: podFailures,
			Signal: SignalDNS},
		Prior: 0.75, InferredMinWorkloads: SharedMinWorkloads,
	}, {
		Name: name, Cause: cause, Link: LinkConfiguresDNS,
		Effect: Side{Kind: kindClusterDNS, Modes: []detection.Mode{
			detection.ModeUnavailable, detection.ModeActiveProbe,
			ModeResolution}},
		Prior: 0.75,
	}}
}

// dnsConfigHops lead from a pod whose lookups fail to the DNS objects
// that changed in the window.
func (v *view) dnsConfigHops(pod inventory.EntityID) []hop {
	if pod.Kind == kindClusterDNS {
		return v.changedDNSHops()
	}
	if v.dnsServer(pod) {
		return nil
	}
	return v.changedDNSHops()
}

// changedDNSHops are the hops to the DNS objects with a change.
func (v *view) changedDNSHops() []hop {
	var out []inventory.EntityID
	for _, id := range v.dnsConfigObjects() {
		if len(v.changesOf(id)) > 0 {
			out = append(out, id)
		}
	}
	return hopsTo(LinkConfiguresDNS, out...)
}

// dnsConfigObjects are the DNS Services and the objects the DNS
// servers' pods are configured by: the ConfigMaps they mount or
// reference, and the roles that bind the accounts they run as, present
// or not.
func (v *view) dnsConfigObjects() []inventory.EntityID {
	out := append([]inventory.EntityID(nil), kube.DNSServices(v.s.Model)...)
	for _, pod := range kube.DNSServerPods(v.s.Model) {
		for _, id := range v.s.Model.Related(pod, inventory.References,
			inventory.Outgoing) {
			if id.Kind == kube.KindConfigMap {
				out = append(out, id)
			}
		}
		for _, account := range v.accountsOf(pod) {
			out = append(out, v.rolesBoundTo(account)...)
		}
	}
	sortIDs(out)
	return slices.Compact(out)
}

// rolesBoundTo are the ClusterRoles that bindings give the account,
// whether or not the role still exists.
func (v *view) rolesBoundTo(account inventory.EntityID) []inventory.EntityID {
	var out []inventory.EntityID
	for _, kind := range []inventory.Kind{kube.KindClusterRoleBinding,
		kube.KindRoleBinding} {
		for _, binding := range v.s.Model.Entities(kind) {
			e, _ := v.s.Model.Entity(binding)
			kindName, name, ok := strings.Cut(
				attrText(e, kube.AttrRBACRole), "/")
			entries := v.bindingAccounts(binding)
			if ok && kindName == "ClusterRole" &&
				subjectsCover(entries, account) {
				out = append(out, inventory.CoreID(
					kube.KindClusterRole, "", name))
			}
		}
	}
	return out
}

// dnsConfigCause reports a candidate blamed as a DNS object. The DNS
// servers read these objects and run healthy by design, and the lookups
// that fail name no object, so what the dependents do and what the
// errors say tell nothing for or against it.
func dnsConfigCause(c *candidate) bool {
	return c.bestMatch().match.row.Link == LinkConfiguresDNS
}
