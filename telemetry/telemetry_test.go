package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/stretchr/testify/assert"
)

func TestNewTelemetry(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Telemetry{Enabled: true}
	telemetry := NewTelemetry(cfg)
	assert.NotNil(telemetry)
	assert.True(telemetry.enabled)

	cfgDisabled := &config.Telemetry{Enabled: false}
	telemetryDisabled := NewTelemetry(cfgDisabled)
	assert.NotNil(telemetryDisabled)
	assert.False(telemetryDisabled.enabled)
}

func TestSendEventDisabled(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Telemetry{Enabled: false}
	telemetry := NewTelemetry(cfg)
	err := telemetry.SendEvent(context.Background(), "test-cluster", "v0.0.1")
	assert.Nil(err)
}

func TestSendEventEnabled(t *testing.T) {
	assert := assert.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal("POST", r.Method)
		assert.Equal("application/json", r.Header.Get("Content-Type"))

		var payload EventPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.Nil(err)
		assert.Equal("test-cluster", payload.ClusterID)
		assert.Equal("v0.0.1", payload.Version)
		assert.NotEmpty(payload.Timestamp)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Telemetry{Enabled: true}
	telemetry := NewTelemetryWithURL(cfg, server.URL)
	err := telemetry.SendEvent(context.Background(), "test-cluster", "v0.0.1")
	assert.Nil(err)
}

func TestSendEventServerError(t *testing.T) {
	assert := assert.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.Telemetry{Enabled: true}
	telemetry := NewTelemetryWithURL(cfg, server.URL)
	err := telemetry.SendEvent(context.Background(), "test-cluster", "v0.0.1")
	assert.Nil(err)
}
