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

func TestRejectSecretConfigRejectsTransportAndDiagnosticsFields(t *testing.T) {
	for _, path := range [][2]string{
		{"app", "proxyURL"},
		{"app", "insecureSkipTLSVerify"},
		{"app", "caBundlePath"},
		{"healthCheck", "pprof"},
		{"healthCheck", "diagnostics"},
		{"auditLog", "output"},
	} {
		spec := map[string]interface{}{
			path[0]: map[string]interface{}{path[1]: "x"},
		}
		assert.ErrorContains(
			t, rejectSecretConfig(spec), path[0]+"."+path[1],
		)
	}
}
