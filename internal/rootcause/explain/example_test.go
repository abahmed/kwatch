package explain_test

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// ExampleExplain solves a tiny cluster: node n1 stops reporting, and
// two pods that run on it fail while a replica on node n2 stays well.
// The solver names the node once, not the two pods.
func ExampleExplain() {
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	relate := func(
		from inventory.EntityID, rel inventory.RelationType,
		to inventory.EntityID,
	) {
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Related, Source: "example", At: start,
			Entity: from, Relation: rel, Targets: []inventory.EntityID{to},
		})
	}
	node := func(name string) inventory.EntityID {
		id := inventory.CoreID(kube.KindNode, "", name)
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "example", At: start,
			Entity: id, Attributes: map[string]inventory.Value{},
		})
		return id
	}
	n1, n2 := node("n1"), node("n2")
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	var pods []inventory.EntityID
	for i, on := range []inventory.EntityID{n1, n1, n2} {
		pod := inventory.CoreID(kube.KindPod, "shop",
			fmt.Sprintf("api-%d", i))
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "example", At: start,
			Entity: pod, Attributes: map[string]inventory.Value{},
		})
		relate(pod, inventory.RunsOn, on)
		relate(pod, inventory.OwnedBy, deploy)
		pods = append(pods, pod)
	}

	findings := map[inventory.EntityID][]detection.Finding{}
	fail := func(
		id inventory.EntityID, mode detection.Mode, after time.Duration,
	) {
		findings[id] = []detection.Finding{{
			Entity: id, Reason: string(mode), Mode: mode,
			Health: detection.Failing, Since: start.Add(after),
			Summary: string(mode),
		}}
	}
	fail(n1, "NotReady", time.Minute)
	fail(pods[0], "NotReady", 2*time.Minute)
	fail(pods[1], "NotReady", 2*time.Minute)
	result := explain.Explain(explain.Snapshot{
		Model: model, Findings: findings, Now: start.Add(10 * time.Minute),
	})
	for _, area := range result.Areas {
		for _, cause := range area.Causes {
			fmt.Println(cause.Root, cause.Row, len(cause.Covers))
		}
	}
	// Output: node//n1 node-not-ready 3
}
