package heartbeat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
)

func TestHeartbeatPingDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			followed.Store(true)
		}))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusFound)
		}))
	defer srv.Close()
	m := NewHeartbeatMonitor(&config.HeartbeatMonitor{
		Enabled: true, URL: srv.URL,
	}, srv.Client(), nil)

	err := m.ping(context.Background())

	require.ErrorIs(t, err, errPingRejected)
	require.False(t, followed.Load())
	require.Nil(t, srv.Client().CheckRedirect,
		"the shared client must not be mutated")
}
