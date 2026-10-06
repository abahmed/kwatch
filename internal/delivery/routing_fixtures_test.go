package delivery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// pagerProvider is a recording provider that skips plain messages, like
// a paging tool.
type pagerProvider struct{ *messageProvider }

func (pagerProvider) SkipsPlainMessages() bool { return true }

// routedSetup is a started manager with a chat provider ("slack") and a
// pager ("pagerduty"), each with its own routes.
type routedSetup struct {
	manager *Manager
	chat    *messageProvider
	pager   *messageProvider
}

// newRoutedSetup starts delivery with the given routes. Pacing is off and
// every provider tries once per round.
func newRoutedSetup(
	t *testing.T, chatRoutes, pagerRoutes []config.AlertRoute,
) routedSetup {
	t.Helper()
	s := newRoutedSetupBeforeStart(t)
	s.setRoutes(chatRoutes, pagerRoutes)
	startManager(t, s.manager)
	return s
}

// setRoutes gives each provider of the unstarted manager its routes.
func (s routedSetup) setRoutes(chat, pager []config.AlertRoute) {
	for name, entry := range s.manager.generation.entries {
		if name == "pagerduty" {
			entry.routes = pager
		} else {
			entry.routes = chat
		}
		s.manager.generation.entries[name] = entry
	}
}

// newRoutedSetupBeforeStart builds the manager without starting it.
func newRoutedSetupBeforeStart(t *testing.T) routedSetup {
	t.Helper()
	chat := newMessageProvider(nil)
	pager := newMessageProvider(nil)
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{
			"slack": {}, "pagerduty": {},
		},
	})
	manager := NewManagerWithDependencies(
		Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.InitRuntime(runtime, func(
		name string, _ map[string]interface{}, _ transport.ProviderContext,
	) Provider {
		if name == "pagerduty" {
			pager.name = name
			return pagerProvider{pager}
		}
		chat.name = name
		return chat
	}))
	for name, entry := range manager.generation.entries {
		entry.retry = retryConfig{maxAttempts: 1, delay: time.Millisecond}
		manager.generation.entries[name] = entry
	}
	manager.pacer.interval = time.Microsecond
	return routedSetup{manager: manager, chat: chat, pager: pager}
}

// routeMsg is a message of conversation key with the given route.
func routeMsg(
	key string, rev int, status notification.Status, route notification.Route,
) notification.Message {
	return notification.Message{
		Key: key, Revision: rev, Status: status, Title: key,
		Note: key, Route: route,
	}
}

// receiveRevision reads messages until revision rev of key arrives. Older
// revisions of a queued conversation may be replaced by newer ones, so
// only the one wanted is waited for.
func receiveRevision(
	t *testing.T, p *messageProvider, key string, rev int,
) notification.Message {
	t.Helper()
	for {
		m := p.receive(t)
		if m.Key == key && m.Revision == rev {
			return m
		}
		require.False(t, m.Key == key && m.Revision > rev,
			"revision %d of %s was skipped", rev, key)
	}
}

// requireNothingMore proves a provider got nothing else: a sentinel
// message every route matches is queued behind, and must come next.
func requireNothingMore(t *testing.T, s routedSetup, p *messageProvider) {
	t.Helper()
	s.manager.NotifyIncident(routeMsg("sentinel", 1,
		notification.StatusCritical, notification.Route{
			Namespaces: []string{"sentinel-ns"},
			Reasons:    []string{"Sentinel"}, Severity: "critical",
		}))
	select {
	case m := <-p.sent:
		require.Equal(t, "sentinel", m.Key,
			"the provider received an unexpected message")
	case <-time.After(10 * time.Second):
		t.Fatal("sentinel never arrived")
	}
}
