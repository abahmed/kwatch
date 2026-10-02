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

func containsError(errs []error, want string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return true
		}
	}
	return false
}

func TestValidatePodNamePatterns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Silences = []SilenceRule{{PodNamePatterns: []string{"api-("}}}
	cfg.IgnorePodNames = []string{"[bad"}
	errs := Validate(cfg)
	if !containsError(errs, "silences[0].podNamePatterns") ||
		!containsError(errs, "ignorePodNames") {
		t.Fatalf("invalid patterns not reported: %v", errs)
	}
	cfg.Silences = []SilenceRule{{PodNamePatterns: []string{"^api-.*"}}}
	cfg.IgnorePodNames = nil
	if containsError(Validate(cfg), "podNamePatterns") {
		t.Fatal("valid pattern rejected")
	}
}

func TestValidateSilenceRulesRejectsEmptyRule(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Silences = []SilenceRule{
		{Namespaces: []string{"batch"}},
		{},
	}
	errs := Validate(cfg)
	if !containsError(errs, "silences[1] sets no matching field") {
		t.Fatalf("empty silence rule not rejected: %v", errs)
	}
	if containsError(errs, "silences[0]") {
		t.Fatalf("non-empty silence rule rejected: %v", errs)
	}
}

func TestValidateAlertHourlyBudget(t *testing.T) {
	cases := map[string]struct {
		value   interface{}
		wantErr bool
	}{
		"zero means unlimited": {0, false},
		"positive integer":     {25, false},
		"float from yaml":      {float64(10), false},
		"negative":             {-1, true},
		"fraction":             {1.5, true},
		"text":                 {"many", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Alert = map[string]map[string]interface{}{
				"slack": {"hourlyBudget": tc.value},
			}
			errs := validateHourlyBudget("slack", cfg.Alert["slack"])
			if (len(errs) > 0) != tc.wantErr {
				t.Fatalf("errs = %v, wantErr %v", errs, tc.wantErr)
			}
		})
	}
}

func TestCompileHourlyBudgetDefaultsAndOverrides(t *testing.T) {
	if got := compileHourlyBudget(nil); got != DefaultHourlyBudget {
		t.Fatalf("default = %d", got)
	}
	got := compileHourlyBudget(map[string]interface{}{"hourlyBudget": 0})
	if got != 0 {
		t.Fatalf("unlimited = %d", got)
	}
	got = compileHourlyBudget(
		map[string]interface{}{"hourlyBudget": float64(7)})
	if got != 7 {
		t.Fatalf("override = %d", got)
	}
}

func TestProviderOptionKeysAcceptHourlyBudget(t *testing.T) {
	if !providerOptionKeys["slack"]["hourlyBudget"] {
		t.Fatal("hourlyBudget must be a known provider option")
	}
}
