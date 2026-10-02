package explain

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestExplainNodeStormHasOneCause fails 1,000 pods of 50 workloads on
// one node: set cover must name the node once, not 50 workloads.
func TestExplainNodeStormHasOneCause(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	f.fail(nodes[0], "NotReady", failingH, 1, "")
	for w := 0; w < 50; w++ {
		pods := f.workload("shop", fmt.Sprintf("app%d", w), 21, nodes[0])
		for _, pod := range pods[:20] {
			f.fail(pod, "NotReady", failingH, 2, "")
		}
		f.relate(pods[20], inventory.RunsOn, nodes[1])
	}
	e := f.explain()
	if got := causes(e); len(got) != 1 || got[0] != "node//n1" {
		t.Fatalf("causes = %v, want [node//n1]", got)
	}
	if n := len(e.Areas[0].Causes[0].Covers); n != 1001 {
		t.Fatalf("node covers %d failures, want 1001", n)
	}
}

// TestExplainRegistryStormHasOneCause fails the pulls of 40 workloads
// from one registry that refuses credentials.
func TestExplainRegistryStormHasOneCause(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	for w := 0; w < 40; w++ {
		name := fmt.Sprintf("app%d", w)
		for _, pod := range f.workload("shop", name, 3, nodes...) {
			f.relate(containerOf(pod), inventory.Pulls, inventory.CoreID(
				kube.KindImage, "", "registry.corp.example/"+name+":1"))
			f.note(pod, "pull: 401 Unauthorized")
			f.fail(containerOf(pod), "ImagePull", failingH, 2, "")
		}
	}
	got := causes(f.explain())
	if len(got) != 1 || got[0] != "registry//registry.corp.example" {
		t.Fatalf("causes = %v, want the registry once", got)
	}
}

// TestExplainKeepsIndependentProblemsApart puts memory pressure on n1
// and a bad rollout on n2 in the same minute: two areas, two causes.
func TestExplainKeepsIndependentProblemsApart(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	f.fail(nodes[0], "MemoryPressure", failingH, 1, "")
	for _, name := range []string{"indexer", "search"} {
		pod := f.workload("batch", name, 1, nodes[0])[0]
		f.fail(pod, "Evicted", degradedH, 2, "")
	}
	checkout := f.workload("shop", "checkout", 2, nodes[1])
	f.change(inventory.CoreID(kube.KindDeployment, "shop", "checkout"),
		1, "spec.template.spec.containers[0].image")
	for _, pod := range checkout {
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, "")
	}
	e := f.explain()
	if len(e.Areas) != 2 {
		t.Fatalf("areas = %d, want 2 (causes %v)", len(e.Areas), causes(e))
	}
	requireCause(t, e, inventory.CoreID(kube.KindPod, "batch",
		"indexer-1-0"), "node//n1")
	requireCause(t, e, containerOf(checkout[0]), "deployment/shop/checkout")
}

// TestExplainDoesNotBlameHealthyNodeForAppCrash crashes an app on a
// node that only has an unrelated disk warning.
func TestExplainDoesNotBlameHealthyNodeForAppCrash(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	f.fail(nodes[0], "Disk.ImageGCFailed", degradedH, 1, "")
	pods := f.workload("shop", "api", 2, nodes...)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 2,
			"panic: assignment to entry in nil map")
	}
	e := f.explain()
	requireCause(t, e, containerOf(pods[0]), "deployment/shop/api")
	requireCause(t, e, containerOf(pods[1]), "deployment/shop/api")
}

// TestExplainDoesNotBlameZoneInSingleZoneCluster fails two nodes of
// the only zone: without a healthy peer elsewhere the zone explains
// nothing more than its nodes.
func TestExplainDoesNotBlameZoneInSingleZoneCluster(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	for _, node := range nodes[:2] {
		f.fail(node, "NotReady", failingH, 1, "")
	}
	e := f.explain()
	for _, root := range causes(e) {
		if root == "zone//zone-a" {
			t.Fatalf("blamed the only zone: %v", causes(e))
		}
	}
	requireCause(t, e, nodes[0], "node//n1")
}

// TestExplainImageTypoDoesNotBlameRegistry rolls out a missing tag in
// one workload while another pulls from the same registry fine.
func TestExplainImageTypoDoesNotBlameRegistry(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1")
	pods := f.workload("shop", "checkout", 2, nodes...)
	healthy := f.workload("shop", "cart", 2, nodes...)
	f.change(inventory.CoreID(kube.KindDeployment, "shop", "checkout"),
		1, "spec.template.spec.containers[0].image")
	for _, pod := range append(pods, healthy...) {
		f.relate(containerOf(pod), inventory.Pulls, inventory.CoreID(
			kube.KindImage, "", "registry.corp.example/"+pod.Name))
	}
	for _, pod := range pods {
		f.note(pod, "manifest unknown: registry.corp.example/checkout:1.2x")
		f.fail(containerOf(pod), "ImagePull", failingH, 2, "")
	}
	requireCause(t, f.explain(), containerOf(pods[0]),
		"deployment/shop/checkout")
}

// TestExplainReportsUnknownBelowFloor leaves one replica of two
// failing with no evidence either way: the cause is unknown.
func TestExplainReportsUnknownBelowFloor(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1")
	pods := f.workload("shop", "api", 2, nodes...)
	f.fail(nodes[0], "NotReady", failingH, 1, "")
	f.fail(pods[0], "CrashLoop", degradedH, 0, "")
	e := f.explain()
	var area Area
	for _, a := range e.Areas {
		if containsID(a.Failures, pods[0]) {
			area = a
		}
	}
	if !area.Unknown() || len(area.Unexplained) != 1 {
		t.Fatalf("area = %+v, want unknown", area)
	}
	if len(area.Trace.Rejected) == 0 {
		t.Fatal("trace lists no rejected candidate")
	}
}

// TestExplainIsDeterministic solves the same snapshot twice.
func TestExplainIsDeterministic(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	for w := 0; w < 5; w++ {
		for _, pod := range f.workload("shop", fmt.Sprint("w", w), 3,
			nodes...) {
			f.fail(containerOf(pod), "CrashLoop", failingH, 2, "")
		}
	}
	a, b := fmt.Sprint(f.explain()), fmt.Sprint(f.explain())
	if a != b {
		t.Fatalf("two solves differ:\n%s\n%s", a, b)
	}
}
