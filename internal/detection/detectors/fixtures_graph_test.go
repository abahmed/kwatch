package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

var t0 = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

func newID(kind inventory.Kind, ns, name string) inventory.EntityID {
	return inventory.EntityID{Kind: kind, Namespace: ns, Name: name}
}

// put observes an entity with attributes at the given time.
func put(m *inventory.Model, id inventory.EntityID, at time.Time,
	attrs map[string]inventory.Value,
) {
	if attrs == nil {
		attrs = map[string]inventory.Value{}
	}
	m.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: at,
		Entity: id, Attributes: attrs,
	})
}

// link records an edge from one entity to another.
func link(m *inventory.Model, from inventory.EntityID,
	rel inventory.RelationType, to inventory.EntityID,
) {
	m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0,
		Entity: from, Relation: rel, Targets: []inventory.EntityID{to},
	})
}

// evaluate runs one detector through a registry so that Since defaults
// and recheck requests are observable. synced nil means every kind.
func evaluate(d detection.Detector, m *inventory.Model, now time.Time,
	id inventory.EntityID, synced func(inventory.Kind) bool,
) detection.Evaluation {
	return detection.NewRegistry(synced, d).Evaluate(m, now, id)
}
