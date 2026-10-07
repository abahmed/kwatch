package delivery

import (
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/notification"
)

// routeSubject is what a route matches on.
type routeSubject struct {
	namespaces []string
	severity   notification.Severity
	reasons    []string
	owners     []string
	// anyOf is set on a summary: it matches when any one of these
	// alternatives, the routes of the problems it names, matches.
	anyOf []routeSubject
	// partial means an empty namespace or reason list is unknown, not
	// "none": the message is a resolve or update of a conversation
	// delivery has no record of.
	partial bool
}

func incidentSubject(m *notification.Message) routeSubject {
	return routeSubject{
		namespaces: m.Route.Namespaces,
		severity:   notification.NormalizeSeverity(m.Route.Severity),
		reasons:    m.Route.Reasons,
		owners:     m.Route.Owners,
		anyOf:      routesOf(m.Route.AnyOf),
	}
}

// matchesRoute requires every constraint the route sets to match at least
// one value of the subject.
func matchesRoute(route config.AlertRoute, subject routeSubject) bool {
	if len(subject.anyOf) > 0 {
		for _, alternative := range subject.anyOf {
			alternative.partial = subject.partial
			if matchesRoute(route, alternative) {
				return true
			}
		}
		return false
	}
	return matchesValues(route.Namespaces, subject.namespaces,
		subject.partial) &&
		matchesSeverity(route.Severities, subject.severity) &&
		matchesValues(route.Reasons, subject.reasons, subject.partial) &&
		matchesValues(route.Owners, subject.owners, subject.partial)
}

// matchesValues is true when nothing is allowed-listed, when one allowed
// value is among the values, or, for a partial subject, when the subject
// names no values at all (they are unknown, not none).
func matchesValues(allowed, values []string, partial bool) bool {
	return len(allowed) == 0 || (partial && len(values) == 0) ||
		anyIn(allowed, values)
}

func matchesSeverity(allowed []string, severity notification.Severity) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, s := range allowed {
		if notification.NormalizeSeverity(s) == severity {
			return true
		}
	}
	return false
}

// anyIn reports whether one allowed value equals one of the values.
// Reasons are compared ignoring case, like runbooks and templates are;
// namespaces never differ by case, so the same comparison is harmless.
func anyIn(allowed, values []string) bool {
	for _, a := range allowed {
		for _, v := range values {
			if strings.EqualFold(a, v) {
				return true
			}
		}
	}
	return false
}

// routedTo reports whether a job goes to a provider with these routes.
// Plain messages are notices and always go everywhere.
func routedTo(routes []config.AlertRoute, job deliverJob) bool {
	switch {
	case len(routes) == 0:
		return true
	case job.kind == jobIncident && job.incident != nil:
		return anyRouteMatches(routes, incidentSubject(job.incident))
	default:
		return true
	}
}

// VerifyAll runs context-aware credential pre-flight on all providers that
// support it.
// Returns a map of provider name → error (nil = verified OK).
