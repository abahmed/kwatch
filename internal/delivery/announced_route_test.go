package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/notification"
)

var (
	oomRoute = []config.AlertRoute{{
		Reasons:    []string{"OOMKilled", "Sentinel"},
		Namespaces: nil,
	}}
	oomNamespaceRoute = notification.Route{
		Namespaces: []string{"shop"}, Reasons: []string{"OOMKilled"},
		Severity: "critical",
	}
	// A resolve has no members left, so the composer writes no reasons.
	bareRoute = notification.Route{
		Namespaces: nil, Reasons: nil, Severity: "critical",
	}
)

// A resolve and every later message of an incident go to the providers
// that received the announcement, even though the resolve carries no
// reasons for the route to match.
func TestResolveReachesProviderRoutedByReason(t *testing.T) {
	s := newRoutedSetup(t, nil, oomRoute)
	s.manager.NotifyIncident(routeMsg("a", 1,
		notification.StatusCritical, oomNamespaceRoute))
	s.manager.NotifyIncident(routeMsg("a", 2,
		notification.StatusCritical, bareRoute))
	s.manager.NotifyIncident(routeMsg("a", 3,
		notification.StatusResolved, bareRoute))

	assert.True(t, receiveRevision(t, s.pager, "a", 3).Resolved())
	assert.True(t, receiveRevision(t, s.chat, "a", 3).Resolved())
}

// A provider the announcement was not routed to does not get the resolve
// either: it never opened an alert.
func TestResolveSkipsProviderThatNeverGotTheAnnouncement(t *testing.T) {
	s := newRoutedSetup(t, nil, oomRoute)
	other := notification.Route{
		Namespaces: []string{"shop"}, Reasons: []string{"Error"},
		Severity: "critical",
	}
	s.manager.NotifyIncident(routeMsg("b", 1,
		notification.StatusCritical, other))
	s.manager.NotifyIncident(routeMsg("b", 2,
		notification.StatusResolved, bareRoute))

	receiveRevision(t, s.chat, "b", 2)
	requireNothingMore(t, s, s.pager)
}

// After a restart delivery has forgotten who got the announcement. A
// resolve that carries no reasons is then judged on what it does carry,
// so a provider routed by reason still hears about it.
func TestResolveOfUnknownConversationIgnoresEmptyReasons(t *testing.T) {
	s := newRoutedSetup(t, nil, oomRoute)
	s.manager.NotifyIncident(routeMsg("c", 5,
		notification.StatusResolved, bareRoute))

	receiveRevision(t, s.pager, "c", 5)
}

// A namespace route is respected for an announcement and kept for the
// resolve of the same conversation.
func TestNamespaceRouteFollowsConversation(t *testing.T) {
	routes := []config.AlertRoute{{Namespaces: []string{"shop"}}}
	s := newRoutedSetup(t, nil, routes)
	s.manager.NotifyIncident(routeMsg("n", 1,
		notification.StatusCritical, oomNamespaceRoute))
	s.manager.NotifyIncident(routeMsg("n", 2,
		notification.StatusResolved, notification.Route{Severity: "critical"}))

	assert.True(t, receiveRevision(t, s.pager, "n", 2).Resolved())
}

func TestRouteReasonsMatchIgnoringCase(t *testing.T) {
	routes := []config.AlertRoute{{Reasons: []string{"oomkilled"}}}
	job := deliverJob{kind: jobIncident, incident: &notification.Message{
		Key: "k", Route: notification.Route{Reasons: []string{"OOMKilled"}},
	}}
	assert.True(t, routedTo(routes, job))
	routes = []config.AlertRoute{{Reasons: []string{"OOMKILLED"}}}
	assert.True(t, routedTo(routes, job))
	routes = []config.AlertRoute{{Reasons: []string{"Evicted"}}}
	assert.False(t, routedTo(routes, job))
}

// A paging-only announcement made before delivery starts goes to the
// pager only, and a resolve that skips paging goes to chat only, exactly
// as after Start.
func TestPendingJobsHonourPagingScope(t *testing.T) {
	s := newRoutedSetupBeforeStart(t)
	pagingOnly := routeMsg("p", 1, notification.StatusCritical,
		notification.Route{Severity: "critical"})
	pagingOnly.PagingOnly = true
	unpaged := routeMsg("u", 2, notification.StatusResolved,
		notification.Route{Severity: "critical"})
	unpaged.SkipPaging = true
	s.manager.NotifyIncident(pagingOnly)
	s.manager.NotifyIncident(unpaged)
	startManager(t, s.manager)

	receiveRevision(t, s.pager, "p", 1)
	receiveRevision(t, s.chat, "u", 2)
	requireNothingMore(t, s, s.pager)
	requireNothingMore(t, s, s.chat)
}

// A job that already names its provider (a restored or replayed one) is
// still checked against the paging scope.
func TestTargetedJobsHonourPagingScope(t *testing.T) {
	s := newRoutedSetup(t, nil, nil)
	msg := routeMsg("t", 1, notification.StatusCritical,
		notification.Route{Severity: "critical"})
	msg.PagingOnly = true
	s.manager.mu.Lock()
	s.manager.fanOut(deliverJob{kind: jobIncident, incident: &msg,
		target: "slack"})
	s.manager.mu.Unlock()

	requireNothingMore(t, s, s.chat)
}

// A summary reaches a provider routed by reason when one of the problems
// it names matches, and not when none does.
func TestSummaryMatchesProviderRoutesThroughItsMembers(t *testing.T) {
	s := newRoutedSetup(t, nil, oomRoute)
	miss := routeMsg("rollup/1", 1, notification.StatusWarning,
		notification.Route{Severity: "warning", AnyOf: []notification.Route{
			{Namespaces: []string{"a"}, Reasons: []string{"Error"},
				Severity: "warning"}}})
	hit := routeMsg("rollup/2", 1, notification.StatusWarning,
		notification.Route{Severity: "warning", AnyOf: []notification.Route{
			{Namespaces: []string{"a"}, Reasons: []string{"Error"}},
			{Namespaces: []string{"b"}, Reasons: []string{"OOMKilled"}}}})
	s.manager.NotifyIncident(miss)
	s.manager.NotifyIncident(hit)

	receiveRevision(t, s.pager, "rollup/2", 1)
	receiveRevision(t, s.chat, "rollup/1", 1)
	// The closing message names no problems; it follows the summary.
	s.manager.NotifyIncident(routeMsg("rollup/2", 2,
		notification.StatusResolved, notification.Route{}))
	s.manager.NotifyIncident(routeMsg("rollup/1", 2,
		notification.StatusResolved, notification.Route{}))
	receiveRevision(t, s.pager, "rollup/2", 2)
	requireNothingMore(t, s, s.pager)
}
