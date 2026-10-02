package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestNewHealthServer(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 8080, Enabled: true}, clock.RealClock{},
	)

	assert.NotNil(t, server)
	assert.Equal(t, 8080, server.port)
	assert.True(t, server.enabled)
}

func TestNewHealthServerDisabled(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 8080, Enabled: false}, clock.RealClock{},
	)

	assert.NotNil(t, server)
	assert.False(t, server.enabled)
}

func TestHealthzHandler(t *testing.T) {
	recorder := httptest.NewRecorder()

	(&HealthServer{}).healthzHandler(recorder,
		httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "OK", recorder.Body.String())
}

func TestHealthHandler(t *testing.T) {
	recorder := httptest.NewRecorder()

	(&HealthServer{}).healthHandler(recorder,
		httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json",
		recorder.Header().Get("Content-Type"))
	var body HealthResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "ok", body.Status)
}

func TestHealthServerStartDisabled(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 0, Enabled: false}, clock.RealClock{},
	)

	require.NoError(t, server.Open())
	assert.Nil(t, server.listener, "a disabled server opens no listener")
	assert.NoError(t, server.Serve(context.Background()))
}

func TestHealthServerServesEndpoints(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 0, Enabled: true}, clock.RealClock{},
	)
	base := serveForTest(t, server)

	status, body := getForTest(t, base+"/healthz")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "OK", body)

	status, _ = getForTest(t, base+"/health")
	assert.Equal(t, http.StatusOK, status)

	status, _ = getForTest(t, base+"/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, status)
}

func TestHealthServerStop(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 0, Enabled: true}, clock.RealClock{},
	)
	require.NoError(t, server.Open())
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()

	require.NoError(t, server.Stop(context.Background()))
	assert.NoError(t, <-done, "Serve returns cleanly after Stop")
}

func TestHealthServerStopNilServer(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 0, Enabled: true}, clock.RealClock{},
	)

	assert.NoError(t, server.Stop(context.Background()))
}
