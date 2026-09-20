package config

import (
	"strings"
	"testing"
)

func TestValidateNamespaceSelector(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NamespaceSelector = "app in ("
	if errs := Validate(cfg); !containsError(errs, "namespaceSelector") {
		t.Fatalf("expected invalid namespace selector error, got %v", errs)
	}

	cfg.NamespaceSelector = "team=platform,environment in (prod,staging)"
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("valid namespace selector rejected: %v", errs)
	}
}

func TestValidateAlertRetrySettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {
			"retry": map[string]interface{}{
				"maxAttempts":   0,
				"delay":         "-1s",
				"maxBackoff":    "not-duration",
				"jitterEnabled": "yes",
				"jitterFactor":  -1,
			},
		},
	}
	errs := Validate(cfg)
	for _, want := range []string{
		"maxAttempts",
		"delay",
		"maxBackoff",
		"jitterEnabled",
		"jitterFactor",
	} {
		if !containsError(errs, want) {
			t.Errorf("expected retry validation error for %s, got %v", want, errs)
		}
	}
}

func TestValidateAlertRetrySettingsAcceptsRuntimeEncodings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {
			"retry": map[string]interface{}{
				"maxAttempts":   float64(5),
				"delay":         "250ms",
				"maxBackoff":    "10s",
				"jitterEnabled": true,
				"jitterFactor":  1,
			},
		},
	}
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("valid retry settings rejected: %v", errs)
	}
}

func TestValidateNodeLeaseStaleSeconds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ClusterResourceMonitor.NodeLeaseStaleSeconds = -1
	if errs := Validate(cfg); !containsError(errs, "nodeLeaseStaleSeconds") {
		t.Fatalf("expected negative Lease threshold error, got %v", errs)
	}

	cfg.ClusterResourceMonitor.NodeLeaseStaleSeconds = 0
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("zero Lease threshold should select the default: %v", errs)
	}
}

func containsError(errs []error, want string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return true
		}
	}
	return false
}
