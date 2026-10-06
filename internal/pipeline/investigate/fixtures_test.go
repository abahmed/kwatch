package investigate

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

var investigatedAt = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// testModel builds a model from observations.
func testModel(
	t *testing.T, observations ...inventory.Observation,
) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	for _, o := range observations {
		if o.At.IsZero() {
			o.At = investigatedAt
		}
		if o.Source == "" {
			o.Source = "test"
		}
		if _, err := m.Apply(o); err != nil {
			t.Fatalf("apply %v: %v", o.Entity, err)
		}
	}
	return m
}

func observed(
	id inventory.EntityID, attrs map[string]inventory.Value,
) inventory.Observation {
	return inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs}
}

func related(
	id inventory.EntityID, rel inventory.RelationType,
	targets ...inventory.EntityID,
) inventory.Observation {
	return inventory.Observation{Kind: inventory.Related, Entity: id,
		Relation: rel, Targets: targets}
}

func changedFields(
	id inventory.EntityID, paths ...string,
) inventory.Observation {
	change := inventory.Change{Entity: id, At: investigatedAt}
	for _, path := range paths {
		change.Fields = append(change.Fields, inventory.FieldChange{
			Path: path, Before: "old-secret-value", After: "new-secret-value"})
	}
	return inventory.Observation{Kind: inventory.Changed, Entity: id,
		Change: change}
}

func text(v string) inventory.Value { return inventory.Text(v) }

func num(v float64) inventory.Value { return inventory.Number(v) }

// incidentOf builds a settling incident of root with members.
func incidentOf(
	root inventory.EntityID, members ...detection.Finding,
) incident.Incident {
	p := incident.Incident{ID: "inc-1", Root: root,
		State: incident.Settling, Opened: investigatedAt,
		Members: map[detection.Key]detection.Finding{}}
	for _, m := range members {
		p.Members[m.Key()] = m
	}
	return p
}

func finding(
	id inventory.EntityID, mode string, evidence ...detection.Evidence,
) detection.Finding {
	return detection.Finding{Entity: id, Reason: mode,
		Mode:     detection.Mode(mode),
		Severity: detection.Critical, Evidence: evidence}
}

// planAndRun plans p with sources and runs the investigation.
func planAndRun(
	t *testing.T, s Sources, p incident.Incident,
) (Investigation, Result) {
	t.Helper()
	plan, ok := NewInvestigator(s).Plan(p)
	if !ok {
		t.Fatal("no investigator planned the incident")
	}
	return plan, Bounded(plan.Run(context.Background()))
}

func evidenceOf(r Result, fact string) []incident.Fact {
	var out []incident.Fact
	for _, e := range r.Evidence {
		if e.Kind == fact {
			out = append(out, e)
		}
	}
	return out
}
