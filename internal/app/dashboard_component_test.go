package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/health"
)

func componentState(deps *serverDeps, name string) health.ComponentStatus {
	return deps.healthServer.ComponentStatuses()[name]
}

// dashboardDeps runs dash under a supervisor the way serve does and
// stops both when the test ends.
func dashboardDeps(
	t *testing.T, dash *dashboard.Server,
) (*serverDeps, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	deps := componentDeps()
	deps.ctx, deps.cancel, deps.dashboardServer = ctx, cancel, dash
	supervisor := newComponentSupervisor(time.Now)
	t.Cleanup(func() {
		cancel()
		_ = dash.Stop(context.Background())
		supervisor.wg.Wait()
	})
	return deps, func() { startDashboard(ctx, deps, supervisor) }
}

// A dashboard port that is taken when kwatch starts is not the end of
// the dashboard: /health shows it degraded, and it serves once the port
// is free, without a restart.
func TestDashboardListenerIsRetriedAndShownOnHealth(t *testing.T) {
	holder, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	port := holder.Addr().(*net.TCPAddr).Port
	dash := dashboard.NewServer(
		dashboard.Options{Token: "tok", Port: port}, clock.RealClock{})
	deps, start := dashboardDeps(t, dash)

	openDashboardEarly(deps.ctx, deps)
	require.NotNil(t, deps.dashboardServer,
		"a failed open dropped the dashboard for good")
	start()

	require.Eventually(t, func() bool {
		return componentState(deps, dashboardComponent).State == "degraded"
	}, 10*time.Second, 10*time.Millisecond,
		"a dashboard that cannot listen is not on /health")
	require.NoError(t, holder.Close())
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	require.Eventually(t, func() bool {
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return componentState(deps, dashboardComponent).State == "running"
	}, 15*time.Second, 20*time.Millisecond,
		"the dashboard did not come back once the port was free")
}

// failingSecretReads makes the first n reads of a Secret fail, as an API
// hiccup at pod start does.
func failingSecretReads(client *fake.Clientset, n int64) {
	var reads atomic.Int64
	client.PrependReactor("get", "secrets",
		func(clienttesting.Action) (bool, runtime.Object, error) {
			if reads.Add(1) <= n {
				return true, nil, errors.New("api server unavailable")
			}
			return false, nil, nil
		})
}

// Credentials that could not be read at start are read again while
// kwatch runs and handed to the running dashboard; until then /health
// shows them missing.
func TestDashboardCredentialsAreReadAgainAfterAFailedStart(t *testing.T) {
	t.Setenv(dashboardSecretEnv, "kwatch-dashboard")
	t.Setenv("POD_NAMESPACE", "kwatch")
	client := fake.NewSimpleClientset(
		emptyDashboardSecret("kwatch-dashboard"))
	// The read at start and the first retry fail.
	failingSecretReads(client, 2)
	token, password := dashboardCredentials(
		context.Background(), client, "kwatch")
	require.Empty(t, token+password, "the read at start must fail")
	dash := dashboard.NewServer(
		dashboard.Options{Token: token, Password: password},
		clock.RealClock{})
	deps, start := dashboardDeps(t, dash)
	deps.clients.Kubernetes = client

	start()

	missing := componentState(deps, dashboardCredentialsComponent)
	require.Equal(t, "degraded", missing.State)
	require.False(t, missing.Available)
	require.Eventually(t, dash.HasCredentials, 15*time.Second,
		20*time.Millisecond, "the credentials were never read again")
	require.Eventually(t, func() bool {
		statuses := deps.healthServer.ComponentStatuses()
		_, shown := statuses[dashboardCredentialsComponent]
		return !shown
	}, 5*time.Second, 10*time.Millisecond,
		"/health still shows the credentials missing")
	secret, err := client.CoreV1().Secrets("kwatch").Get(
		context.Background(), "kwatch-dashboard", metav1.GetOptions{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Authorization",
		"Bearer "+string(secret.Data[dashboardSecretToken]))
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code,
		"the stored token does not open the dashboard")
}

// Credentials an operator left empty in the environment are a choice:
// nothing is retried and nothing is degraded.
func TestEmptyEnvironmentCredentialsAreNotRetried(t *testing.T) {
	t.Setenv(dashboardSecretEnv, "")
	dash := dashboard.NewServer(dashboard.Options{}, clock.RealClock{})
	deps, start := dashboardDeps(t, dash)
	deps.clients.Kubernetes = fake.NewSimpleClientset()

	start()

	statuses := deps.healthServer.ComponentStatuses()
	_, shown := statuses[dashboardCredentialsComponent]
	require.False(t, shown)
}
