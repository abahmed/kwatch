package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

const resolverLookup = "dial tcp: lookup db on 169.254.20.10:53: read " +
	"udp 10.244.1.5:48762->169.254.20.10:53: i/o timeout"

// resolverCase has two workloads failing lookups on an address that a
// third, failing workload names in its own error.
func resolverCase(f *fixture) inventory.EntityID {
	agent := f.workload("kube-system", "node-local-dns", 1)[0]
	f.fail(containerOf(agent), "CrashLoop", failingH, 1,
		"[FATAL] listen tcp 169.254.20.10:8080: bind: address "+
			"already in use")
	var first inventory.EntityID
	for _, name := range []string{"cart", "invoices"} {
		pod := f.workload("shop", name, 1)[0]
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, resolverLookup)
		if first.Name == "" {
			first = containerOf(pod)
		}
	}
	return first
}

func TestResolverHolderIsBlamedNotTheClusterDNS(t *testing.T) {
	f := newFixture(t)
	effect := resolverCase(f)

	c := requireCause(t, f.explain(), effect,
		"deployment/kube-system/node-local-dns")
	if c.Row != RowResolverWorkload {
		t.Fatalf("row = %s, want %s", c.Row, RowResolverWorkload)
	}
}

var resolverRowCases = []rowCase{
	{row: RowResolverWorkload,
		want:  "deployment/kube-system/node-local-dns",
		build: resolverCase},
}
