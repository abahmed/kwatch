package app

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
)

func refusesConnections(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return true
	}
	_ = conn.Close()
	return false
}

// The shutdown budget counts one healthStopTimeout for both listeners, so
// they must stop at the same time: a health server that is slow to stop
// must not delay the dashboard's stop. Here the health server cannot
// finish stopping (a client holds a connection open) until the dashboard
// has stopped.
func TestShutdownStopsBothListenersTogether(t *testing.T) {
	now := clock.RealClock{}
	healthPort, dashPort := freeTCPPort(t), freeTCPPort(t)
	healthAddr := "127.0.0.1:" + strconv.Itoa(healthPort)
	dashAddr := "127.0.0.1:" + strconv.Itoa(dashPort)
	healthServer := health.NewHealthServerWithClock(
		config.HealthCheck{Enabled: true, Port: healthPort}, now)
	require.NoError(t, healthServer.Open())
	go func() { _ = healthServer.Serve(context.Background()) }()
	dash := dashboard.NewServer(
		dashboard.Options{Token: "tok", Port: dashPort}, now)
	require.NoError(t, dash.Open(t.Context()))
	go func() { _ = dash.Serve(context.Background()) }()
	deps := newServerDeps(t.Context(), func() {}, &bootstrap{
		entitlements: feature.ProEntitlements(),
		healthServer: healthServer, dashboardServer: dash,
		deliveryManager: &delivery.Manager{}, clock: now,
	})

	// A new connection that sends nothing keeps the health server's
	// shutdown waiting. The request after it proves the server accepted
	// the held connection first.
	held, err := net.Dial("tcp", healthAddr)
	require.NoError(t, err)
	defer func() { _ = held.Close() }()
	resp, err := http.Get("http://" + healthAddr + "/healthz")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	done := make(chan struct{})
	go func() {
		defer close(done)
		stopServersAtExit(deps, false)
	}()
	require.Eventually(t, func() bool {
		return refusesConnections(dashAddr)
	}, healthStopTimeout-time.Second, 10*time.Millisecond,
		"the dashboard waited for the health server to stop")
	require.NoError(t, held.Close())
	<-done
}

// The listeners' stop is one step of the budget, and the whole sequence
// still fits the termination grace period.
func TestShutdownBudgetCountsOneListenerStop(t *testing.T) {
	require.Equal(t, healthStopTimeout+backgroundShutdownTimeout+
		deliveryStopTimeout+leaseReleaseTimeout, totalShutdownBudget)
	require.Less(t, totalShutdownBudget, terminationGracePeriod)
}
