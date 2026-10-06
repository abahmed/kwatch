package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// maxSummaryRoutes bounds the alternatives a summary carries. Past it the
// rest are folded into one route that holds all their namespaces and
// reasons, which matches a little more than it should, never less.
const maxSummaryRoutes = 64

// summaryRoute is the route of a message that names several problems: it
// matches a provider's route when the route would match any one of them,
// so a provider routed by reason or namespace is not left out of the
// startup summary, roll-up, outage message or digest that holds a
// problem it would be sent on its own. The severity is the highest of
// the problems, for providers that list only the loudest.
func summaryRoute(
	decisions []incident.Decision, risks []detection.Finding,
) notification.Route {
	var alternatives []notification.Route
	for _, d := range decisions {
		alternatives = append(alternatives,
			route(d.Incident, sortedMembers(d.Incident)))
	}
	for _, risk := range risks {
		r := notification.Route{Severity: "info",
			Reasons: []string{risk.Reason}}
		if risk.Entity.Namespace != "" {
			r.Namespaces = []string{risk.Entity.Namespace}
		}
		alternatives = append(alternatives, r)
	}
	alternatives = distinctRoutes(alternatives)
	if len(alternatives) > maxSummaryRoutes {
		alternatives = append(alternatives[:maxSummaryRoutes-1],
			unionRoute(alternatives[maxSummaryRoutes-1:]))
	}
	return notification.Route{Severity: loudest(alternatives),
		AnyOf: alternatives}
}

func distinctRoutes(routes []notification.Route) []notification.Route {
	seen := map[string]bool{}
	var out []notification.Route
	for _, r := range routes {
		id := r.Severity + "|" + strings.Join(r.Namespaces, ",") + "|" +
			strings.Join(r.Reasons, ",")
		if !seen[id] {
			seen[id] = true
			out = append(out, r)
		}
	}
	return out
}

func unionRoute(routes []notification.Route) notification.Route {
	namespaces, reasons := map[string]bool{}, map[string]bool{}
	union := notification.Route{Severity: loudest(routes)}
	for _, r := range routes {
		for _, ns := range r.Namespaces {
			namespaces[ns] = true
		}
		for _, reason := range r.Reasons {
			reasons[reason] = true
		}
	}
	union.Namespaces, union.Reasons = sortedSet(namespaces), sortedSet(reasons)
	return union
}

// loudest is the highest severity of the routes: critical, warning, info.
func loudest(routes []notification.Route) string {
	best := "info"
	for _, r := range routes {
		switch r.Severity {
		case "critical":
			return "critical"
		case "warning":
			best = "warning"
		}
	}
	return best
}
