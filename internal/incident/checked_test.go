package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// An incident without a cause remembers which kinds upstream of its
// failures were checked and found healthy, once each, sorted.
func TestManagerKeepsCheckedKinds(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	node := entity(kube.KindNode, "n1")
	image := inventory.CoreID(kube.KindImage, "", "registry/app:1")
	r.rule.checked = map[inventory.EntityID][]inventory.EntityID{
		web.Entity: {node, image, node},
	}

	r.raise(at(0), web)

	got := r.only()
	if len(got.Checked) != 2 || got.Checked[0] != string(kube.KindImage) ||
		got.Checked[1] != string(kube.KindNode) {
		t.Fatalf("checked = %v, want [image node]", got.Checked)
	}
}

// A placement names the checked kinds from the area's trace.
func TestCheckedKindsAreSortedAndUnique(t *testing.T) {
	ids := []inventory.EntityID{
		entity(kube.KindNode, "n1"), entity(kube.KindNode, "n2"),
		entity(kube.KindConfigMap, "settings"),
	}
	got := checkedKinds(ids)
	if len(got) != 2 || got[0] != "configmap" || got[1] != "node" {
		t.Fatalf("kinds = %v", got)
	}
}

// The incident remembers the alternatives the solver weighed, for the
// audit log.
func TestManagerKeepsConsideredAlternatives(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.rule.alternatives = map[inventory.EntityID][]explain.Cause{
		web.Entity: {
			{Root: entity(kube.KindNode, "n1"), Row: "node-not-ready",
				Confidence: 0.42},
		},
	}

	r.raise(at(0), web)

	got := r.only()
	want := entity(kube.KindNode, "n1").String() + " (node-not-ready, 0.42)"
	if len(got.Considered) != 1 || got.Considered[0] != want {
		t.Fatalf("considered = %v, want [%s]", got.Considered, want)
	}
}
