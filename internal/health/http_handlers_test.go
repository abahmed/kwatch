package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestSetReady(t *testing.T) {
	h := &HealthServer{}
	assert.False(t, h.ready.Load())
	h.SetReady(true)
	assert.True(t, h.ready.Load())
	h.SetReady(false)
	assert.False(t, h.ready.Load())
}

func TestHealthServerStopIsIdempotent(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{Port: 0, Enabled: true}, clock.RealClock{},
	)
	assert.NoError(t, startForTest(server))
	assert.NoError(t, server.Stop(context.Background()))
	assert.NoError(t, server.Stop(context.Background()))
	assert.Error(t, startForTest(server))
}

func TestReadyzHandlerNotReady(t *testing.T) {
	h := &HealthServer{}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h.readyzHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	body := make([]byte, 32)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "not ready", string(body[:n]))
}

func TestReadyzHandlerReady(t *testing.T) {
	h := &HealthServer{}
	h.SetReady(true)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h.readyzHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 8)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "OK", string(body[:n]))
}

func TestAvailabilityzAcceptsStartingAndLeader(t *testing.T) {
	h := &HealthServer{}
	req := httptest.NewRequest(http.MethodGet, "/availabilityz", nil)
	w := httptest.NewRecorder()
	h.availabilityzHandler(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode)

	h.SetLeadership(LeadershipStatus{Role: "starting"})
	w = httptest.NewRecorder()
	h.availabilityzHandler(w, req)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	h.SetLeadership(LeadershipStatus{Role: "stopped"})
	w = httptest.NewRecorder()
	h.availabilityzHandler(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode)
}

func TestServeMuxExposesOnlyOperationalEndpoints(t *testing.T) {
	h := NewHealthServerWithClock(
		config.HealthCheck{Port: 0, Enabled: true}, clock.RealClock{},
	)
	ts := httptest.NewServer(newServeMux(h))
	defer ts.Close()

	for _, path := range []string{
		"/healthz", "/readyz", "/availabilityz", "/health", "/metrics",
	} {
		resp, err := http.Get(ts.URL + path)
		assert.NoError(t, err)
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, path)
		resp.Body.Close()
	}
	for _, path := range []string{
		"/incidents", "/test-alert", "/deadletters", "/kubelet",
		"/telemetry", "/security", "/controlplane", "/informer",
		"/debug/pprof/", "/debug/pprof/heap",
	} {
		resp, err := http.Get(ts.URL + path)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
		resp.Body.Close()
	}
}
