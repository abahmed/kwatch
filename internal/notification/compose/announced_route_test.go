package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// After a restart delivery has forgotten who heard the announcement. The
// resolve has no members, so its route comes from the route the
// incident kept (AnnouncedRoute), not from what is left of it.
func TestResolveRouteComesFromTheAnnouncedRoute(t *testing.T) {
	p := scaledToZero()
	p.Members = nil
	p.Tier = incident.Digest
	p.AnnouncedRoute = &incident.AnnouncedRoute{
		Namespaces: []string{"shop"}, Reasons: []string{"ScaledToZeroRouted"},
		Severity: "critical",
	}

	msg := Writer{}.Write(incident.Decision{Action: incident.Resolve,
		Incident: p, Reason: "healthy for 5m0s"}, writerNow)

	assert.Equal(t, []string{"shop"}, msg.Route.Namespaces)
	assert.Equal(t, []string{"ScaledToZeroRouted"}, msg.Route.Reasons)
	assert.Equal(t, "critical", msg.Route.Severity,
		"the severity the alert was opened with")
}

// An update keeps the announced names and adds what is failing now.
func TestUpdateRouteAddsLiveMembersToTheAnnouncedRoute(t *testing.T) {
	p := scaledToZero()
	p.AnnouncedRoute = &incident.AnnouncedRoute{
		Namespaces: []string{"billing"}, Reasons: []string{"OOMKilled"},
		Severity: "warning",
	}

	msg := Writer{}.Write(incident.Decision{Action: incident.Update,
		Incident: p, Reason: incident.ReasonMaterialChange}, writerNow)

	assert.Equal(t, []string{"billing", "shop"}, msg.Route.Namespaces)
	assert.Len(t, msg.Route.Reasons, 2)
}
