package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

type namespaceScope string

func (s namespaceScope) Allows(_ inventory.Reader, sig detection.Finding) bool {
	return sig.Entity.Namespace == string(s)
}

func scopedIncident(memberNS, rootNS string) incident.Incident {
	member := detection.Finding{Entity: inventory.EntityID{
		Kind: "pod", Namespace: memberNS, Name: "p"}, Reason: "Error"}
	root := detection.Finding{Entity: inventory.EntityID{
		Kind: "pvc", Namespace: rootNS, Name: "data"}, Reason: "Pending"}
	return incident.Incident{
		Members: map[detection.Key]detection.Finding{member.Key(): member},
		Cause:   &rootcause.CauseRecord{RootFindings: []detection.Finding{root}},
	}
}

func TestIncidentScopeMatchesAnyMemberOrRootFinding(t *testing.T) {
	inScope := IncidentScope(nil, namespaceScope("shop"))

	assert.True(t, inScope(scopedIncident("shop", "other")))
	assert.True(t, inScope(scopedIncident("other", "shop")))
	assert.False(t, inScope(scopedIncident("other", "other")))
	// A resolve carries no members and often no root findings; only a
	// delivered announcement keeps it in scope.
	assert.False(t, inScope(incident.Incident{}))
	assert.True(t, inScope(incident.Incident{Scope: incident.ScopeIn}))
	assert.False(t, inScope(incident.Incident{Scope: incident.ScopeOut}))
}

func TestEngineDropsOutOfScopeDecisions(t *testing.T) {
	a := &announcer{scope: IncidentScope(nil, namespaceScope("shop"))}
	decisions := []incident.Decision{
		{Incident: scopedIncident("shop", "")},
		{Incident: scopedIncident("dev", "")},
	}

	kept := a.inScope(decisions)

	assert.Len(t, kept, 1)
	a.scope = nil
	assert.Len(t, a.inScope(decisions), 2)
}

func TestEngineRecordsAnnouncementScope(t *testing.T) {
	e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
		func(d *Dependencies) {
			d.InScope = IncidentScope(nil, namespaceScope("shop"))
		})
	resolve := incident.Decision{
		Action:   incident.Resolve,
		Incident: incident.Incident{Scope: incident.ScopeIn},
	}
	dropped := incident.Decision{Action: incident.Resolve}

	kept := e.announcer.inScope([]incident.Decision{resolve, dropped})

	assert.Len(t, kept, 1)
}
