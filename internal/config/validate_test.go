package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAppTransportSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.ProxyURL = "not a URL"
	errs := Validate(cfg)
	assertErrorContains(t, errs, "app.proxyURL must be a valid URL")

	path := t.TempDir() + "/ca.pem"
	require.NoError(t, os.WriteFile(path, []byte("not PEM"), 0600))
	cfg.App.ProxyURL = "http://proxy.example.test:8080"
	cfg.App.CABundlePath = path
	errs = Validate(cfg)
	assertErrorContains(
		t,
		errs,
		"app.caBundlePath does not contain a valid PEM certificate",
	)
}

func assertErrorContains(t *testing.T, errs []error, want string) {
	t.Helper()
	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return
		}
	}
	t.Fatalf("expected validation error containing %q, got %v", want, errs)
}

func TestInvalidSeverityKeys(t *testing.T) {
	m := map[string]string{
		"ImagePullBackOff": "medium",
		"Evicted":          "warning",
		"OOMKilled":        "severe", // typo'd
		"CrashLoopBackOff": "critical",
		"BackOff":          "HIGH", // case variant is valid
	}
	assert.Equal(t, []string{"OOMKilled"}, InvalidSeverityKeys(m))
}

func TestValidateSeverityValues(t *testing.T) {
	cfg := &Config{
		SeverityByReason: map[string]string{
			"ImagePullBackOff": "medium",
			"OOMKilled":        "boom",
		},
		SeverityByOwnerKind: map[string]string{
			"StatefulSet": "high",
			"DaemonSet":   "urgent",
		},
	}
	errs := ValidateConfig(cfg)
	require.Contains(t, errs, `severityByReason["OOMKilled"] has invalid severity "boom" (expected one of critical, high, medium, warning, normal)`)
	require.Contains(t, errs, `severityByOwnerKind["DaemonSet"] has invalid severity "urgent" (expected one of critical, high, medium, warning, normal)`)

	verrs := Validate(cfg)
	var foundReason, foundKind bool
	for _, e := range verrs {
		if strings.Contains(e.Error(), `severityByReason["OOMKilled"]`) {
			foundReason = true
		}
		if strings.Contains(e.Error(), `severityByOwnerKind["DaemonSet"]`) {
			foundKind = true
		}
	}
	assert.True(t, foundReason, "Validate must flag invalid severityByReason value")
	assert.True(t, foundKind, "Validate must flag invalid severityByOwnerKind value")
}

func TestValidateAcceptsCaseVariantSeverity(t *testing.T) {
	cfg := &Config{
		Alert:            map[string]map[string]interface{}{"slack": {}},
		Workers:          1,
		SeverityByReason: map[string]string{"ImagePullBackOff": "High"},
	}
	cfg.Correlation.Window = 10
	cfg.Correlation.LifecycleInterval = 60
	assert.Empty(t, ValidateConfig(cfg), "case-variant severity values are accepted")
}
