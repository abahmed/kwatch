package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemovedReasonWarningsNameEveryPlace(t *testing.T) {
	cfg := &Config{
		Reasons:          []string{"!ResourceHigh", "OOMKilled"},
		SeverityByReason: map[string]string{"Killed": "critical"},
		Templates:        map[string]string{"containercannotrun": "x"},
		Runbooks:         map[string]string{"ResourceCritical": "u"},
		Silences: []SilenceRule{{Reasons: []string{
			"DeploymentAvailableFalse", "deploymentavailablefalse"}}},
	}

	got := removedReasonWarnings(cfg)

	require.Equal(t, []string{
		"reason `ResourceHigh` in reasons was removed in this release; " +
			"this rule matches nothing",
		"reason `Killed` in severityByReason was removed in this " +
			"release; this rule matches nothing",
		"reason `ContainerCannotRun` in templates was removed in this " +
			"release; this rule matches nothing",
		"reason `ResourceCritical` in runbooks was removed in this " +
			"release; this rule matches nothing",
		"reason `DeploymentAvailableFalse` in silences[0].reasons was " +
			"removed in this release; this rule matches nothing",
	}, got)
	assert.Subset(t, Warnings(cfg), got, "logged at startup")
	err := RemovedReasonsError(cfg)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "ResourceHigh"))
}

func TestRemovedReasonWarningsQuietForCurrentReasons(t *testing.T) {
	cfg := &Config{Reasons: []string{"OOMKilled", "CrashLoopBackOff"},
		Silences: []SilenceRule{{Reasons: []string{"LivenessKilled"}}}}

	assert.Empty(t, removedReasonWarnings(cfg))
	assert.NoError(t, RemovedReasonsError(cfg))
	assert.NoError(t, RemovedReasonsError(nil))
}

// The silences built from the legacy ignore* fields are not the
// file's: only the listed ones are checked.
func TestRemovedReasonWarningsSkipSyntheticSilences(t *testing.T) {
	cfg := &Config{Silences: []SilenceRule{
		{Reasons: []string{"Killed"}}, {Reasons: []string{"Killed"}},
	}, syntheticSilences: 1}

	got := removedReasonWarnings(cfg)

	require.Len(t, got, 1)
	assert.Contains(t, got[0], "silences[0]")
}
