package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

func ownedPod(owner string) incident.Incident {
	p := podCrash(incident.Notify)
	p.Owner = owner
	return p
}

func TestAnnouncementRouteCarriesTheOwner(t *testing.T) {
	d := incident.Decision{Action: incident.Announce,
		Incident: ownedPod("payments"), Reason: incident.ReasonSettled}

	msg := Writer{}.Write(d, writerNow)

	assert.Equal(t, []string{"payments"}, msg.Route.Owners)
}

func TestIncidentWithoutAnOwnerRoutesWithNone(t *testing.T) {
	d := incident.Decision{Action: incident.Announce,
		Incident: ownedPod(""), Reason: incident.ReasonSettled}

	msg := Writer{}.Write(d, writerNow)

	assert.Empty(t, msg.Route.Owners)
}

func TestResolveRouteKeepsTheAnnouncedOwners(t *testing.T) {
	p := ownedPod("search")
	p.Members = nil
	p.AnnouncedRoute = &incident.AnnouncedRoute{
		Namespaces: []string{"default"}, Severity: "warning",
		Owners: []string{"payments"},
	}

	msg := Writer{}.Write(incident.Decision{Action: incident.Resolve,
		Incident: p, Reason: "healthy for 5m0s"}, writerNow)

	assert.Equal(t, []string{"payments"}, msg.Route.Owners,
		"the resolve goes where the announcement went")
}

func TestUpdateRouteAddsTheLiveOwner(t *testing.T) {
	p := ownedPod("search")
	p.AnnouncedRoute = &incident.AnnouncedRoute{
		Namespaces: []string{"default"}, Severity: "warning",
		Owners: []string{"payments"},
	}

	msg := Writer{}.Write(incident.Decision{Action: incident.Update,
		Incident: p, Reason: incident.ReasonMaterialChange}, writerNow)

	assert.Equal(t, []string{"payments", "search"}, msg.Route.Owners)
}

func TestSummaryRouteListsTheOwnersOfItsProblems(t *testing.T) {
	a := incident.Decision{Incident: ownedPod("payments")}
	b := incident.Decision{Incident: ownedPod("search")}
	c := incident.Decision{Incident: ownedPod("")}

	route := summaryRoute([]incident.Decision{a, b, c}, nil)

	var owners [][]string
	for _, alternative := range route.AnyOf {
		owners = append(owners, alternative.Owners)
	}
	assert.ElementsMatch(t,
		[][]string{{"payments"}, {"search"}, nil}, owners)
}
