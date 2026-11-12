package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// nodeCovers reports whether the node candidate covers effect after
// the candidates are pruned.
func nodeCovers(f *fixture, node, effect inventory.EntityID) bool {
	v := newView(f.snapshot())
	cs := v.candidates(failures(f.findings))
	c, ok := cs.byID[node]
	if !ok {
		return false
	}
	_, ok = c.covers[effect]
	return ok
}

// A crash loop that began before the node came under pressure is not
// the node's.
func TestNodeDoesNotClaimFailureThatPredatesIt(t *testing.T) {
	for _, tc := range []struct {
		podMinutes int
		want       bool
	}{{1, false}, {6, true}} {
		f := newFixture(t)
		nodes := f.nodes("zone-a", "n1")
		pod := f.workload("shop", "api", 1, nodes...)[0]
		f.fail(nodes[0], detection.ModePressureStall, degradedH, 4, "")
		f.fail(containerOf(pod), detection.ModeCrashLoop, failingH,
			tc.podMinutes, "")
		if got := nodeCovers(f, nodes[0], containerOf(pod)); got != tc.want {
			t.Errorf("pod failing at +%dm: node covers = %v, want %v",
				tc.podMinutes, got, tc.want)
		}
	}
}

// A workload that fails the same way on a healthy node is not the
// node's.
func TestNodeDoesNotClaimWorkloadFailingOnHealthyNode(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("shop", "api", 2, nodes...)
	f.fail(nodes[0], detection.ModePressureStall, degradedH, 1, "")
	for _, pod := range pods {
		f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 3, "")
	}
	if nodeCovers(f, nodes[0], containerOf(pods[0])) {
		t.Fatal("the node claims a crash loop that also runs on n2")
	}
}

// A container whose last run stopped at start with its own fatal line
// is not the node's.
func TestNodeDoesNotClaimSelfEvidentFailure(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1")
	pod := f.workload("shop", "api", 1, nodes...)[0]
	f.fail(nodes[0], detection.ModePressureStall, degradedH, 1, "")
	startFails(f, []inventory.EntityID{pod}, configLine, 1)
	f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 3, "")
	if nodeCovers(f, nodes[0], containerOf(pod)) {
		t.Fatal("the node claims a crash with its own fatal line")
	}
	if !newView(f.snapshot()).selfEvident(pod) {
		t.Fatal("the fatal line is not seen")
	}
}
