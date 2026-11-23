package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dnsServingRowCases covers the DNS server workload blamed for lookups
// that fail while its pods run.
var dnsServingRowCases = []rowCase{
	{row: RowDNSServing, want: "deployment/kube-system/coredns",
		build: func(f *fixture) inventory.EntityID {
			for _, server := range f.workload("kube-system", "coredns", 2) {
				f.setAttrs(server, attrs{
					kube.AttrReady: inventory.Bool(true)})
			}
			var first inventory.EntityID
			for _, name := range []string{"cart", "invoices"} {
				pod := f.workload("shop", name, 1)[0]
				f.fail(containerOf(pod), "CrashLoop", failingH, 2,
					"lookup api.partner.example on 10.96.0.10:53: "+
						"server misbehaving")
				if first.Name == "" {
					first = containerOf(pod)
				}
			}
			return first
		}},
}
