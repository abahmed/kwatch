package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

var t0 = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

func newID(kind knowledge.Kind, ns, name string) knowledge.EntityID {
	return knowledge.EntityID{Kind: kind, Namespace: ns, Name: name}
}

// put observes an entity with attributes at the given time.
func put(m *knowledge.Model, id knowledge.EntityID, at time.Time,
	attrs map[string]knowledge.Value,
) {
	if attrs == nil {
		attrs = map[string]knowledge.Value{}
	}
	m.Apply(knowledge.Fact{
		Kind: knowledge.Observed, Source: "test", At: at,
		Entity: id, Attributes: attrs,
	})
}

// link records an edge from one entity to another.
func link(m *knowledge.Model, from knowledge.EntityID,
	rel knowledge.RelationType, to knowledge.EntityID,
) {
	m.Apply(knowledge.Fact{
		Kind: knowledge.Related, Source: "test", At: t0,
		Entity: from, Relation: rel, Targets: []knowledge.EntityID{to},
	})
}

// evaluate runs one detector through a registry so that Since defaults
// and recheck requests are observable. synced nil means every kind.
func evaluate(d signal.Detector, m *knowledge.Model, now time.Time,
	id knowledge.EntityID, synced func(knowledge.Kind) bool,
) signal.Evaluation {
	return signal.NewRegistry(synced, d).Evaluate(m, now, id)
}
