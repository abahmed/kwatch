package delivery

import (
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func matchesRoute(route config.AlertRoute, inc *model.Incident) bool {
	if len(route.Namespaces) > 0 {
		found := false
		for _, ns := range route.Namespaces {
			if ns == inc.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(route.Severities) > 0 {
		found := false
		for _, s := range route.Severities {
			if model.NormalizeSeverity(s) == inc.Severity {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(route.Reasons) > 0 {
		found := false
		for _, r := range route.Reasons {
			if r == inc.Reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// shouldDeliver checks whether an incident should be delivered to a provider.
// If the provider has no routes defined, all incidents are delivered.

func shouldDeliver(routes []config.AlertRoute, inc *model.Incident) bool {
	if len(routes) == 0 {
		return true
	}
	for _, route := range routes {
		if matchesRoute(route, inc) {
			return true
		}
	}
	return false
}

// VerifyAll runs context-aware credential pre-flight on all providers that
// support it.
// Returns a map of provider name → error (nil = verified OK).
