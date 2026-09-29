package reason

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

var t0 = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

func eid(kind knowledge.Kind, ns, name string) knowledge.EntityID {
	return knowledge.NewEntityID(kind, ns, name)
}

func podID(name string) knowledge.EntityID {
	return eid(kube.KindPod, "ns", name)
}

func (h *harness) observe(
	id knowledge.EntityID, attrs map[string]knowledge.Value,
) {
	if _, err := h.model.Apply(knowledge.Fact{
		Kind: knowledge.Observed, Entity: id, At: h.now,
		Source: "test", Attributes: attrs,
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) relate(
	from knowledge.EntityID, rel knowledge.RelationType,
	to ...knowledge.EntityID,
) {
	if _, err := h.model.Apply(knowledge.Fact{
		Kind: knowledge.Related, Entity: from, Relation: rel,
		Targets: to, Source: "test", At: h.now,
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) change(
	id knowledge.EntityID, at time.Time, created bool,
	fields ...knowledge.FieldChange,
) {
	if _, err := h.model.Apply(knowledge.Fact{
		Kind: knowledge.Changed, Entity: id, At: at,
		Change: knowledge.Change{
			Entity: id, At: at, Created: created, Fields: fields,
		},
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) evidenceSignal(
	id knowledge.EntityID, reason, summary string, since time.Time,
	ev ...signal.Evidence,
) signal.Signal {
	s := signal.Signal{
		Entity: id, Reason: reason, Since: since,
		Severity: signal.Critical, Summary: summary, Evidence: ev,
	}
	h.signals[id] = append(h.signals[id], s)
	return s
}

func field(path, before, after string) knowledge.FieldChange {
	return knowledge.FieldChange{Path: path, Before: before, After: after}
}

// ownedPod wires pod -> owner and registers both as observed.
func (h *harness) ownedPod(pod, owner knowledge.EntityID) {
	h.observe(pod, nil)
	h.observe(owner, nil)
	h.relate(pod, knowledge.OwnedBy, owner)
}
