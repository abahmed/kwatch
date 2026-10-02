package config

import (
	"strings"
	"testing"
)

func routeConfig(route map[string]interface{}) *Config {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"webhook": "x", "routes": []interface{}{route}},
	}
	return cfg
}

func TestValidateAlertRoutesSeverities(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		valid    bool
	}{
		{name: "critical", severity: "critical", valid: true},
		{name: "warning any case", severity: "Warning", valid: true},
		{name: "info", severity: "info", valid: true},
		{name: "incident-only high", severity: "high"},
		{name: "typo", severity: "critcal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := routeConfig(map[string]interface{}{
				"severities": []interface{}{tc.severity},
			})
			rejected := containsError(Validate(cfg),
				"alert.slack.routes[0].severities")
			if rejected == tc.valid {
				t.Fatalf("severity %q rejected = %v", tc.severity, rejected)
			}
		})
	}
}

func TestWarningsFlagUnknownRouteKeys(t *testing.T) {
	cfg := routeConfig(map[string]interface{}{
		"namespace":  []interface{}{"shop"},
		"severities": []interface{}{"critical"},
	})
	var found []string
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "routes[0]") {
			found = append(found, warning)
		}
	}
	if len(found) != 1 ||
		!strings.Contains(found[0], "alert.slack.routes[0].namespace ") {
		t.Fatalf("route warnings = %v", found)
	}
}
