package health

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

// An in-process rebuild binds the same port again right after Stop, even
// when the old server never reached Serve.
func TestStopFreesThePortForTheNextGeneration(t *testing.T) {
	cfg := config.HealthCheck{Port: freePort(t), Enabled: true}
	first := NewHealthServerWithClock(cfg, clock.RealClock{})
	require.NoError(t, first.Open())

	second := NewHealthServerWithClock(cfg, clock.RealClock{})
	require.Error(t, second.Open(), "port must be held until Stop")

	require.NoError(t, first.Stop(context.Background()))
	require.NoError(t, second.Open())
	require.NoError(t, second.Stop(context.Background()))
}
