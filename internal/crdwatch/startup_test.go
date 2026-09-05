package crdwatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRejectSecretConfigAllowsOrdinarySettings(t *testing.T) {
	err := rejectSecretConfig(map[string]interface{}{
		"workers": float64(2),
		"healthCheck": map[string]interface{}{
			"enabled": true,
		},
	})

	assert.NoError(t, err)
}

func TestRejectSecretConfigRejectsAlert(t *testing.T) {
	err := rejectSecretConfig(map[string]interface{}{
		"alert": map[string]interface{}{},
	})

	assert.ErrorContains(t, err, "spec.alert is forbidden")
}

func TestRejectSecretConfigRejectsGlobalSecretFields(t *testing.T) {
	for _, spec := range []map[string]interface{}{
		{
			"healthCheck": map[string]interface{}{
				"diagnosticsToken": "token",
			},
		},
		{
			"heartbeatMonitor": map[string]interface{}{
				"url": "https://heartbeat.example.test/token",
			},
		},
	} {
		assert.Error(t, rejectSecretConfig(spec))
	}
}

func TestNormalizeLegacySpecUsesNestedThreshold(t *testing.T) {
	spec := map[string]interface{}{
		"pendingPodThreshold": float64(120),
	}
	normalizeLegacySpec(spec)

	assert.NotContains(t, spec, "pendingPodThreshold")
	assert.Equal(
		t,
		float64(120),
		spec["pendingPodMonitor"].(map[string]interface{})["threshold"],
	)
}

func TestNormalizeLegacySpecDoesNotOverrideNestedThreshold(t *testing.T) {
	spec := map[string]interface{}{
		"pendingPodThreshold": float64(120),
		"pendingPodMonitor": map[string]interface{}{
			"threshold": float64(30),
		},
	}
	normalizeLegacySpec(spec)

	assert.Equal(
		t,
		float64(30),
		spec["pendingPodMonitor"].(map[string]interface{})["threshold"],
	)
}
