package heartbeat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
)

func TestHeartbeatDisabled(t *testing.T) {
	cfg := &config.HeartbeatMonitor{Enabled: false}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Start should return immediately without blocking or panicking
	m.Start(ctx)
}

func TestHeartbeatNoURL(t *testing.T) {
	cfg := &config.HeartbeatMonitor{Enabled: true, URL: ""}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m.Start(ctx)
}

func TestHeartbeatPing(t *testing.T) {
	var pingCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pingCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.HeartbeatMonitor{Enabled: true, URL: srv.URL}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient)

	// call ping directly (not via ticker)
	m.ping(context.Background())

	assert.Equal(t, int32(1), pingCount.Load(), "should have sent one ping")
}

func TestHeartbeatPingHTTPerror(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := &config.HeartbeatMonitor{Enabled: true, URL: srv.URL}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient)

	m.ping(context.Background())
}

func TestHeartbeatStartStopsWhenContextIsCanceled(t *testing.T) {
	cfg := &config.HeartbeatMonitor{
		Enabled:  true,
		URL:      "http://heartbeat.invalid",
		Interval: 1,
	}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient)
	if NewHeartbeatMonitorWithRuntime(
		config.RuntimeConfigFor(&config.Config{
			HeartbeatMonitor: *cfg,
		}), http.DefaultClient,
	) == nil {
		t.Fatal("runtime constructor returned nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

func TestHeartbeatPingHandlesMissingClientAndInvalidURL(t *testing.T) {
	cfg := &config.HeartbeatMonitor{
		Enabled: true,
		URL:     "http://heartbeat.invalid",
	}
	NewHeartbeatMonitor(cfg, nil).ping(context.Background())
	NewHeartbeatMonitor(&config.HeartbeatMonitor{
		Enabled: true,
		URL:     "://invalid",
	}, http.DefaultClient).ping(context.Background())
}
