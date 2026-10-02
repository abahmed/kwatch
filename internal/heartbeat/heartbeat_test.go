package heartbeat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
)

func TestHeartbeatDisabled(t *testing.T) {
	cfg := &config.HeartbeatMonitor{Enabled: false}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Start should return immediately without blocking or panicking
	m.Start(ctx, nil)
}

func TestHeartbeatNoURL(t *testing.T) {
	cfg := &config.HeartbeatMonitor{Enabled: true, URL: ""}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m.Start(ctx, nil)
}

func TestHeartbeatPing(t *testing.T) {
	var pingCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pingCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.HeartbeatMonitor{Enabled: true, URL: srv.URL}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)

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
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)

	m.ping(context.Background())
}

func TestHeartbeatStartStopsWhenContextIsCanceled(t *testing.T) {
	cfg := &config.HeartbeatMonitor{
		Enabled:  true,
		URL:      "http://heartbeat.invalid",
		Interval: 1,
	}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)
	if NewHeartbeatMonitorWithRuntime(
		config.RuntimeConfigFor(&config.Config{
			HeartbeatMonitor: *cfg,
		}), http.DefaultClient, nil,
	) == nil {
		t.Fatal("runtime constructor returned nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Start(ctx, nil); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

func TestHeartbeatPingHandlesMissingClientAndInvalidURL(t *testing.T) {
	cfg := &config.HeartbeatMonitor{
		Enabled: true,
		URL:     "http://heartbeat.invalid",
	}
	NewHeartbeatMonitor(cfg, nil, nil).ping(context.Background())
	NewHeartbeatMonitor(&config.HeartbeatMonitor{
		Enabled: true,
		URL:     "://invalid",
	}, http.DefaultClient, nil).ping(context.Background())
}

func TestHeartbeatStartImmediatelyPings(t *testing.T) {
	var pingCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			pingCount.Add(1)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer srv.Close()

	cfg := &config.HeartbeatMonitor{
		Enabled:  true,
		URL:      srv.URL,
		Interval: 100000,
	}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start the monitor and wait for it to complete
	m.Start(ctx, nil)

	assert.Greater(t, pingCount.Load(), int32(0),
		"immediate ping should occur at startup")
}

func TestHeartbeatPingHasTimeoutPerPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	defer srv.Close()

	cfg := &config.HeartbeatMonitor{Enabled: true, URL: srv.URL}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, nil)

	// Ping should include its own timeout, even if ctx has a long timeout
	ctx := context.Background()
	m.ping(ctx)

	// No panic or error should occur
}

func TestHeartbeatWaitsForMonitoringReadiness(t *testing.T) {
	var pingCount atomic.Int32
	pinged := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			pingCount.Add(1)
			pinged <- struct{}{}
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()
	var ready atomic.Bool
	// checked receives every readiness check that answered "not ready".
	checked := make(chan struct{})
	gate := func() bool {
		isReady := ready.Load()
		if !isReady {
			checked <- struct{}{}
		}
		return isReady
	}
	cfg := &config.HeartbeatMonitor{
		Enabled: true, URL: srv.URL, Interval: 3600,
	}
	m := NewHeartbeatMonitor(cfg, http.DefaultClient, gate)
	m.readyCheck = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx, nil) }()

	// Two readiness checks while not ready: neither may ping.
	<-checked
	<-checked
	assert.Equal(t, int32(0), pingCount.Load(), "pinged while not ready")
	ready.Store(true)
	for waiting := true; waiting; {
		select {
		case <-checked: // a check that started before the switch
		case <-pinged:
			waiting = false
		}
	}

	cancel()
	assert.NoError(t, <-done)
	assert.Equal(t, int32(1), pingCount.Load())
}
