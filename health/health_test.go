package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/stretchr/testify/assert"
)

func TestNewHealthServer(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	assert.NotNil(server)
	assert.Equal(8080, server.port)
	assert.True(server.enabled)
}

func TestNewHealthServerDisabled(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: false})
	assert.NotNil(server)
	assert.Equal(8080, server.port)
	assert.False(server.enabled)
}

func TestHealthzHandler(t *testing.T) {
	assert := assert.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &HealthServer{}
		h.healthzHandler(w, r)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)

	body := make([]byte, 100)
	n, _ := resp.Body.Read(body)
	assert.Equal("OK", string(body[:n]))
}

func TestHealthHandler(t *testing.T) {
	assert := assert.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &HealthServer{}
		h.healthHandler(w, r)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)
	assert.Equal("application/json", resp.Header.Get("Content-Type"))

	var healthResp HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	assert.Nil(err)
	assert.Equal("ok", healthResp.Status)
}

func TestHealthServerStartDisabled(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: false})
	err := server.Start(context.Background())
	assert.Nil(err)
}

func TestHealthServerStartEnabled(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	err := server.Start(context.Background())
	assert.Nil(err)

	// Test /healthz endpoint
	resp, err := http.Get("http://localhost:8080/healthz")
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)

	// Test /health endpoint
	resp, err = http.Get("http://localhost:8080/health")
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)

	server.Stop(context.Background())
}

func TestHealthServerStop(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	err := server.Start(context.Background())
	assert.Nil(err)

	err = server.Stop(context.Background())
	assert.Nil(err)
}

func TestHealthServerStopNilServer(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	err := server.Stop(context.Background())
	assert.Nil(err)
}
