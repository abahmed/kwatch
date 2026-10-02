package heartbeat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestHeartbeatReportsPingOutcome(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusServiceUnavailable)
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(int(status.Load()))
		}))
	defer srv.Close()
	m := NewHeartbeatMonitor(&config.HeartbeatMonitor{
		Enabled: true, URL: srv.URL,
	}, srv.Client(), nil)
	before := metrics.DefaultRegistry().HeartbeatFailures.Load()
	var reported []error

	m.reportPing(m.ping(context.Background()), func(err error) {
		reported = append(reported, err)
	})
	status.Store(http.StatusOK)
	m.reportPing(m.ping(context.Background()), func(err error) {
		reported = append(reported, err)
	})

	require.Len(t, reported, 2)
	require.ErrorIs(t, reported[0], errPingRejected)
	require.NoError(t, reported[1])
	require.Equal(t, int64(1),
		metrics.DefaultRegistry().HeartbeatFailures.Load()-before)
}

func TestHeartbeatStartReportsFirstPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()
	m := NewHeartbeatMonitor(&config.HeartbeatMonitor{
		Enabled: true, URL: srv.URL, Interval: 60,
	}, srv.Client(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	reports := make(chan error, 1)
	done := make(chan error, 1)

	go func() {
		done <- m.Start(ctx, func(err error) { reports <- err })
	}()

	select {
	case err := <-reports:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("first ping was not reported")
	}
	cancel()
	require.NoError(t, <-done)
}
