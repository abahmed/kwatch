package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
