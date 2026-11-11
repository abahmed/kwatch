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
)

func TestHeartbeatRetriesAFailedPingBeforeTheNextInterval(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		}))
	defer srv.Close()
	m := NewHeartbeatMonitor(&config.HeartbeatMonitor{
		Enabled: true, URL: srv.URL, Interval: 3600,
	}, srv.Client(), nil)
	m.readyCheck = time.Millisecond
	m.retryInitial, m.retryMax = 5*time.Millisecond, 10*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx, nil) }()

	require.Eventually(t, func() bool { return calls.Load() >= 2 },
		5*time.Second, time.Millisecond,
		"a failed ping waited a whole interval")
	cancel()
	require.NoError(t, <-done)
}

func TestHeartbeatRetryWaitDoublesToItsCap(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &HeartbeatMonitor{
		retryInitial: 10 * time.Second, retryMax: time.Minute,
		now: func() time.Time { return now },
	}
	var r retrySchedule
	var waits []time.Duration
	for i := 0; i < 5; i++ {
		r.after(errPingFailed, m, time.Hour)
		waits = append(waits, r.at.Sub(now))
	}
	require.Equal(t, []time.Duration{
		10 * time.Second, 20 * time.Second, 40 * time.Second,
		time.Minute, time.Minute}, waits)

	r.after(nil, m, time.Hour)
	require.False(t, r.due(now.Add(time.Hour)), "success clears retries")
	r.after(errPingFailed, m, 3*time.Second)
	require.Equal(t, 3*time.Second, r.at.Sub(now),
		"the wait never exceeds the interval")
}
