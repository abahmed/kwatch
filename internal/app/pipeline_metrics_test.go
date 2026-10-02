package app

import (
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestCountDecisionCountsActions(t *testing.T) {
	reg := &metrics.Registry{}

	for _, d := range []incident.Decision{
		{Action: incident.Announce},
		{Action: incident.Resolve},
		{Reason: "startup summary"},
	} {
		countDecision(reg, d)
	}

	if got := reg.IncidentActions[0].Load(); got != 1 {
		t.Fatalf("announce count = %d", got)
	}
	if got := reg.IncidentActions[2].Load(); got != 1 {
		t.Fatalf("resolve count = %d", got)
	}
}

func TestCountDecisionIgnoresActionlessDecisions(t *testing.T) {
	reg := &metrics.Registry{}

	if countDecision(reg, incident.Decision{}) {
		t.Fatal("actionless decision was counted")
	}
	for i := range reg.IncidentActions {
		if reg.IncidentActions[i].Load() != 0 {
			t.Fatal("actionless decision was counted")
		}
	}
}

func TestRefreshOpenIncidentsCountsUnresolved(t *testing.T) {
	reg := &metrics.Registry{}
	state := []incident.Incident{
		{State: incident.Open}, {State: incident.Resolved},
		{State: incident.Recovering},
	}

	refreshOpenIncidents(reg, func() []incident.Incident { return state })

	if got := reg.IncidentsOpen.Load(); got != 2 {
		t.Fatalf("open incidents = %d", got)
	}
}
