package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const dnsLookupText = "dial tcp: lookup db.shop.svc.cluster.local on " +
	"10.96.0.10:53: i/o timeout"

var (
	dnsConfigMap = inventory.CoreID(kube.KindConfigMap, "kube-system",
		"coredns")
	dnsRole    = inventory.CoreID("clusterrole", "", "system:coredns")
	dnsService = inventory.CoreID(kube.KindService, "kube-system",
		"kube-dns")
)

var dnsConfigRowCases = []rowCase{
	{row: "dns-config-changed", want: dnsConfigMap.String(),
		build: func(f *fixture) inventory.EntityID {
			return dnsClientCase(f, dnsConfigMap)
		}},
	{row: "dns-role-changed", want: dnsRole.String(),
		build: func(f *fixture) inventory.EntityID {
			return dnsClientCase(f, dnsRole)
		}},
	{row: "dns-service-changed", want: dnsService.String(),
		build: func(f *fixture) inventory.EntityID {
			return dnsClientCase(f, dnsService)
		}},
	{row: "dns-config-changed-resolver", want: dnsConfigMap.String(),
		build: func(f *fixture) inventory.EntityID {
			return dnsProbeCase(f, dnsConfigMap)
		}},
	{row: "dns-role-changed-resolver", want: dnsRole.String(),
		build: func(f *fixture) inventory.EntityID {
			return dnsProbeCase(f, dnsRole)
		}},
	{row: "dns-service-changed-resolver", want: dnsService.String(),
		build: func(f *fixture) inventory.EntityID {
			return dnsProbeCase(f, dnsService)
		}},
}

// dnsClientCase edits a DNS object, then fails the lookups of two
// workloads.
func dnsClientCase(f *fixture, object inventory.EntityID) inventory.EntityID {
	f.add(object)
	f.change(object, 1, "metadata")
	if object == dnsConfigMap {
		// The DNS servers read the ConfigMap.
		for _, pod := range f.workload("kube-system", "coredns", 1) {
			f.relate(pod, inventory.References, object)
		}
	}
	pods := dnsVictims(f, dnsLookupText, "a", "b")
	return pods[0]
}

// dnsProbeCase edits a DNS object, then fails the DNS probe.
func dnsProbeCase(f *fixture, object inventory.EntityID) inventory.EntityID {
	f.add(object)
	f.change(object, 1, "metadata")
	f.fail(kube.ClusterDNS, "Unavailable.CoreDNS", failingH, 1,
		dnsLookupText)
	return kube.ClusterDNS
}
