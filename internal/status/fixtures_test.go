package status

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var testNow = time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)

// testModel returns a model; apply feeds it observations.
func testModel(t *testing.T) *inventory.Model {
	t.Helper()
	return inventory.NewModel(inventory.Options{})
}

func observe(
	t *testing.T, m *inventory.Model, id inventory.EntityID,
	attrs map[string]inventory.Value,
) {
	t.Helper()
	_, err := m.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: testNow,
		Entity: id, Attributes: attrs,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func relate(
	t *testing.T, m *inventory.Model, from inventory.EntityID,
	relation inventory.RelationType, to ...inventory.EntityID,
) {
	t.Helper()
	_, err := m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: testNow,
		Entity: from, Relation: relation, Targets: to,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// addNode adds a node in a zone, ready or not.
func addNode(
	t *testing.T, m *inventory.Model, name, zone string, ready bool,
) inventory.EntityID {
	t.Helper()
	id := inventory.CoreID(kube.KindNode, "", name)
	observe(t, m, id, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(ready)})
	zoneID := inventory.CoreID(kube.KindZone, "", zone)
	observe(t, m, zoneID, nil)
	relate(t, m, id, inventory.PartOf, zoneID)
	return id
}

// addPod adds a pod running on a node.
func addPod(
	t *testing.T, m *inventory.Model, name string, node inventory.EntityID,
) inventory.EntityID {
	t.Helper()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "shop",
		Name: name}
	observe(t, m, id, nil)
	relate(t, m, id, inventory.RunsOn, node)
	return id
}
