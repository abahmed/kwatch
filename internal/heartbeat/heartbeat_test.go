package heartbeat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestHeartbeatDisabled(t *testing.T) {
	cfg := &config.HeartbeatMonitor{Enabled: false}
	m := NewHeartbeatMonitor(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Start should return immediately without blocking or panicking
	m.Start(ctx)
}

func TestHeartbeatNoURL(t *testing.T) {
	cfg := &config.HeartbeatMonitor{Enabled: true, URL: ""}
	m := NewHeartbeatMonitor(cfg)

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
	m := NewHeartbeatMonitor(cfg)

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
	m := NewHeartbeatMonitor(cfg)

	m.ping(context.Background())
}
