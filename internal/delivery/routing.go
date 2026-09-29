package delivery

import (
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/notice"
)

// routeSubject is what a route matches on, for incidents and stories.
type routeSubject struct {
	namespaces []string
	severity   model.Severity
	reasons    []string
}

func incidentSubject(inc *model.Incident) routeSubject {
	return routeSubject{
		namespaces: []string{inc.Namespace}, severity: inc.Severity,
		reasons: []string{inc.Reason},
	}
}

func storySubject(m *notice.Message) routeSubject {
	return routeSubject{
		namespaces: m.Route.Namespaces,
		severity:   model.NormalizeSeverity(m.Route.Severity),
		reasons:    m.Route.Reasons,
	}
}

// matchesRoute requires every constraint the route sets to match at least
// one value of the subject.
func matchesRoute(route config.AlertRoute, subject routeSubject) bool {
	if len(route.Namespaces) > 0 &&
		!anyIn(route.Namespaces, subject.namespaces) {
		return false
	}
	if len(route.Severities) > 0 {
		found := false
		for _, s := range route.Severities {
			found = found || model.NormalizeSeverity(s) == subject.severity
		}
		if !found {
			return false
		}
	}
	return len(route.Reasons) == 0 || anyIn(route.Reasons, subject.reasons)
}

func anyIn(allowed, values []string) bool {
	for _, a := range allowed {
		for _, v := range values {
			if a == v {
				return true
			}
		}
	}
	return false
}

// routedTo reports whether a job goes to a provider with these routes.
// Plain messages and events are notices and always go everywhere.
func routedTo(routes []config.AlertRoute, job deliverJob) bool {
	switch {
	case len(routes) == 0:
		return true
	case job.kind == jobIncident:
		return shouldDeliver(routes, job.inc)
	case job.kind == jobStory:
		subject := storySubject(job.story)
		for _, route := range routes {
			if matchesRoute(route, subject) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// shouldDeliver checks whether an incident should be delivered to a provider.
// If the provider has no routes defined, all incidents are delivered.

func shouldDeliver(routes []config.AlertRoute, inc *model.Incident) bool {
	if len(routes) == 0 {
		return true
	}
	for _, route := range routes {
		if matchesRoute(route, incidentSubject(inc)) {
			return true
		}
	}
	return false
}

// VerifyAll runs context-aware credential pre-flight on all providers that
// support it.
// Returns a map of provider name → error (nil = verified OK).
