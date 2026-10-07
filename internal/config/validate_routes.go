package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// knownRouteKeys are the fields a provider route understands.
var knownRouteKeys = map[string]bool{
	"namespaces": true, "severities": true, "reasons": true,
	"owners": true,
}

// validateAlertRoutes rejects route severities that no message ever has.
// Such a route matches nothing, so the provider would silently receive no
// incidents at all.
func validateAlertRoutes(cfg *Config) []error {
	var errs []error
	forEachRoute(cfg, func(provider string, i int, route map[string]any) {
		for _, severity := range runtimeStringList(route["severities"]) {
			if notification.IsRouteSeverity(severity) {
				continue
			}
			errs = append(errs, fmt.Errorf(
				"alert.%s.routes[%d].severities has unknown severity %q "+
					"(expected one of %s)", provider, i, severity,
				strings.Join(notification.RouteSeverities, ", "),
			))
		}
	})
	return errs
}

// routeWarnings reports route keys kwatch does not read, such as a typo of
// "namespaces". The route still loads; the key is ignored.
func routeWarnings(cfg *Config) []string {
	var warnings []string
	forEachRoute(cfg, func(provider string, i int, route map[string]any) {
		keys := make([]string, 0, len(route))
		for key := range route {
			if !knownRouteKeys[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			warnings = append(warnings, fmt.Sprintf(
				"alert.%s.routes[%d].%s is not a route field and is "+
					"ignored (use namespaces, severities, reasons or owners)",
				provider, i, key))
		}
	})
	return warnings
}

// forEachRoute visits every route mapping of every provider in a stable
// order.
func forEachRoute(
	cfg *Config, visit func(provider string, i int, route map[string]any),
) {
	providers := make([]string, 0, len(cfg.Alert))
	for name := range cfg.Alert {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		routes, _ := cfg.Alert[provider]["routes"].([]interface{})
		for i, raw := range routes {
			if route, ok := raw.(map[string]interface{}); ok {
				visit(provider, i, route)
			}
		}
	}
}
