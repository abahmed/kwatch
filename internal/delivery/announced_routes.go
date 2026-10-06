package delivery

import (
	"sync"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/notification"
)

// A resolve, and every later message of an incident, carries what is left
// of the incident, not what it was: a resolve has no failing members, so
// it names no reasons. A provider routed by reason or namespace would
// match the announcement and then never hear about the resolve, and its
// alert would stay open.
//
// The routeLedger remembers, per conversation key, which providers were
// sent a message of it. A later message goes to those providers whatever
// its route says. A conversation the ledger has never seen (kwatch
// restarted since) is judged on the route it carries, ignoring the parts
// it leaves empty, because empty means "unknown" there.

// maxTrackedConversations bounds the ledger. The oldest conversation is
// forgotten first.
const maxTrackedConversations = 4096

// routeLedger maps conversation key to the providers it was routed to.
type routeLedger struct {
	mu    sync.Mutex
	told  map[string]map[string]struct{}
	order []string
}

// record notes that the conversation was routed to the providers named.
// With no providers it still notes that the conversation exists, so a
// later resolve is not mistaken for one of a conversation delivery never
// saw.
func (l *routeLedger) record(key string, providers ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.told == nil {
		l.told = make(map[string]map[string]struct{})
	}
	set, known := l.told[key]
	if !known {
		set = make(map[string]struct{})
		l.told[key] = set
		l.order = append(l.order, key)
		if len(l.order) > maxTrackedConversations {
			delete(l.told, l.order[0])
			l.order = l.order[1:]
		}
	}
	for _, name := range providers {
		set[name] = struct{}{}
	}
}

// forget drops the conversation, so a later message under the same key is
// judged as the first of a new one.
func (l *routeLedger) forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, known := l.told[key]; !known {
		return
	}
	delete(l.told, key)
	for i, k := range l.order {
		if k == key {
			l.order = append(l.order[:i], l.order[i+1:]...)
			break
		}
	}
}

// lookup reports whether the conversation is known, and whether the
// provider was sent a message of it.
func (l *routeLedger) lookup(key, provider string) (known, told bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	set, known := l.told[key]
	_, told = set[provider]
	return known, told
}

// wants reports whether a job goes to the provider: its routes match the
// job, or the provider already received a message of the conversation.
// Plain messages go everywhere.
func (m *Manager) wants(entry providerEntry, job deliverJob) bool {
	if len(entry.routes) == 0 || job.kind != jobIncident ||
		job.incident == nil {
		return true
	}
	known, told := m.told.lookup(job.key(), entry.lookupName())
	if told || routedTo(entry.routes, job) {
		return true
	}
	if known || job.incident.IsOpening() {
		return false
	}
	subject := incidentSubject(job.incident)
	subject.partial = true
	return anyRouteMatches(entry.routes, subject)
}

// noteRouted records which providers a job was queued for.
func (m *Manager) noteRouted(job deliverJob, providers ...string) {
	if job.kind == jobIncident && job.incident != nil {
		m.told.record(job.key(), providers...)
	}
}

// anyRouteMatches reports whether one of the routes matches the subject.
func anyRouteMatches(
	routes []config.AlertRoute, subject routeSubject,
) bool {
	for _, route := range routes {
		if matchesRoute(route, subject) {
			return true
		}
	}
	return false
}

// routesOf turns the alternatives of a summary into subjects.
func routesOf(alternatives []notification.Route) []routeSubject {
	out := make([]routeSubject, 0, len(alternatives))
	for _, route := range alternatives {
		out = append(out, routeSubject{
			namespaces: route.Namespaces,
			severity:   notification.NormalizeSeverity(route.Severity),
			reasons:    route.Reasons,
		})
	}
	return out
}
