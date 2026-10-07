package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/notification"
)

func ownerRoute(owners ...string) config.AlertRoute {
	return config.AlertRoute{Owners: owners}
}

func TestOwnerRouteMatchesTheOwnerIgnoringCase(t *testing.T) {
	subject := routeSubject{owners: []string{"Payments"}}

	assert.True(t, matchesRoute(ownerRoute("payments"), subject))
	assert.False(t, matchesRoute(ownerRoute("search"), subject))
	assert.True(t, matchesRoute(ownerRoute("search", "payments"), subject))
}

func TestOwnerlessIncidentTakesTheRoutesWithoutOwners(t *testing.T) {
	subject := routeSubject{namespaces: []string{"shop"}}

	assert.False(t, matchesRoute(ownerRoute("payments"), subject),
		"an owner route wants an owner")
	assert.True(t, matchesRoute(config.AlertRoute{}, subject),
		"the default route takes whatever has no owner route")
}

func TestOwnerAndNamespaceMustBothMatch(t *testing.T) {
	route := config.AlertRoute{Namespaces: []string{"shop"},
		Owners: []string{"payments"}}

	assert.True(t, matchesRoute(route, routeSubject{
		namespaces: []string{"shop"}, owners: []string{"payments"}}))
	assert.False(t, matchesRoute(route, routeSubject{
		namespaces: []string{"ops"}, owners: []string{"payments"}}))
}

func TestOwnerOfASummaryMatchesWhenOneProblemDoes(t *testing.T) {
	subject := routeSubject{anyOf: []routeSubject{
		{owners: []string{"search"}}, {owners: []string{"payments"}}}}

	assert.True(t, matchesRoute(ownerRoute("payments"), subject))
	assert.False(t, matchesRoute(ownerRoute("billing"), subject))
}

func TestUnknownOwnerOfAPartialSubjectMatches(t *testing.T) {
	subject := routeSubject{partial: true}

	assert.True(t, matchesRoute(ownerRoute("payments"), subject),
		"a resolve that carries no owner is unknown, not ownerless")
}

func TestIncidentSubjectCarriesTheMessageOwners(t *testing.T) {
	msg := notification.Message{Route: notification.Route{
		Owners: []string{"payments"},
		AnyOf:  []notification.Route{{Owners: []string{"search"}}},
	}}

	subject := incidentSubject(&msg)

	assert.Equal(t, []string{"payments"}, subject.owners)
	assert.Equal(t, []string{"search"}, subject.anyOf[0].owners)
}

func TestOwnerRoutesSendEachIncidentToItsOwnersProvider(t *testing.T) {
	s := newRoutedSetup(t, []config.AlertRoute{ownerRoute("payments")},
		nil)

	s.manager.NotifyIncident(routeMsg("other", 1,
		notification.StatusWarning, notification.Route{
			Owners: []string{"search"}, Severity: "warning"}))
	s.manager.NotifyIncident(routeMsg("mine", 1,
		notification.StatusWarning, notification.Route{
			Owners: []string{"payments"}, Severity: "warning"}))

	got := s.chat.receive(t)
	assert.Equal(t, "mine", got.Key)
}
