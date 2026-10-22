package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func TestIncidentTakesAWithheldProCause(t *testing.T) {
	p := &Incident{}
	p.takeCause(placement{proCause: true})
	if !p.ProCause {
		t.Fatal("a withheld shared factor must mark the incident")
	}
	p.takeCause(placement{})
	if !p.ProCause {
		t.Fatal("a later placement must not forget it")
	}
}

func TestAreaPlacerMarksWithheldFailures(t *testing.T) {
	failure := inventory.CoreID("Pod", "shop", "api-1")
	other := inventory.CoreID("Pod", "shop", "web-1")
	s := explain.Snapshot{Model: inventory.NewModel(inventory.Options{})}
	a := newAreaPlacer(s, explain.Area{
		Failures: []inventory.EntityID{failure, other},
		Withheld: []inventory.EntityID{failure}})
	if !a.placementOf(failure).proCause {
		t.Fatal("withheld failure not marked")
	}
	if a.placementOf(other).proCause {
		t.Fatal("other failure marked")
	}
}
