package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// typoLookup is a pod that cannot resolve a misspelt external name.
const typoLookup = "dial tcp: lookup api.typo-vendor.com on " +
	"10.96.0.10:53: no such host"

// dnsVictims makes one pod of each named workload crash with text.
func dnsVictims(
	f *fixture, text string, names ...string,
) []inventory.EntityID {
	nodes := f.nodes("zone-a", "n1", "n2")
	// The Service the lookups name exists: only DNS can fail them.
	f.add(inventory.CoreID(kube.KindService, "shop", "db"))
	var out []inventory.EntityID
	for _, name := range names {
		pod := f.workload("shop", name, 1, nodes...)[0]
		f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2,
			text)
		out = append(out, containerOf(pod))
	}
	f.workload("shop", "ok", 3, nodes...)
	return out
}

// requireNotDNS fails when cluster DNS is blamed for an effect.
func requireNotDNS(t *testing.T, e Explanation, effects ...inventory.EntityID) {
	t.Helper()
	for _, effect := range effects {
		if c, ok := e.CauseOf(effect); ok && c.Root.Kind == kindClusterDNS {
			t.Fatalf("%s blamed on cluster DNS (%s)", effect, c.Row)
		}
	}
}

// TestExplainOnePodLookupIsNotClusterDNS: one app failing to resolve a
// name is that app's problem; DNS read from its own error is not a
// shared outage.
func TestExplainOnePodLookupIsNotClusterDNS(t *testing.T) {
	f := newFixture(t)
	effects := dnsVictims(f, "dial tcp: lookup db.shop.svc.cluster."+
		"local on 10.96.0.10:53: no such host", "a")
	requireNotDNS(t, f.explain(), effects...)
}

// TestExplainExternalTypoIsNotClusterDNS: an external name that does
// not exist was answered by DNS; it is the name's problem, however many
// apps ask for it.
func TestExplainExternalTypoIsNotClusterDNS(t *testing.T) {
	for _, names := range [][]string{{"a"}, {"a", "b"}} {
		f := newFixture(t)
		effects := dnsVictims(f, typoLookup, names...)
		requireNotDNS(t, f.explain(), effects...)
	}
}

// TestExplainClusterNameLookupsBlameClusterDNS: several apps failing to
// resolve in-cluster names still point at cluster DNS.
func TestExplainClusterNameLookupsBlameClusterDNS(t *testing.T) {
	f := newFixture(t)
	effects := dnsVictims(f, "dial tcp: lookup db.shop.svc.cluster."+
		"local on 10.96.0.10:53: no such host", "a", "b")
	requireCause(t, f.explain(), effects[0], "cluster-dns//cluster-dns")
}

// TestClusterDNSSkipsUnknownExternalNames: "no such host" for a name
// outside the cluster is DNS answering, not DNS failing.
func TestClusterDNSSkipsUnknownExternalNames(t *testing.T) {
	cases := map[string]bool{
		typoLookup: false,
		"dial tcp: lookup db.shop.svc.cluster.local on 10.96.0.10:53: " +
			"no such host": true,
		"dial tcp: lookup api.example.com on 10.96.0.10:53: read udp " +
			"10.0.0.1:4000->10.96.0.10:53: i/o timeout": true,
	}
	for text, want := range cases {
		f := newFixture(t)
		effect := dnsVictims(f, text, "a")[0]
		v := newView(f.snapshot())
		modes := v.virtualModes(kube.ClusterDNS, effect, LinkResolvesVia)
		if got := len(modes) > 0; got != want {
			t.Errorf("%q: resolution failing = %v, want %v", text, got,
				want)
		}
	}
}
