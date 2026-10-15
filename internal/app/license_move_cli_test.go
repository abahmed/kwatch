package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/license"
)

// memLease keeps the lease in memory for a test manager.
type memLease struct {
	lease string
	clock license.ClockGuard
}

func (m *memLease) LoadLease() (string, license.ClockGuard, error) {
	return m.lease, m.clock, nil
}

func (m *memLease) SaveLease(l string, c license.ClockGuard) error {
	m.lease, m.clock = l, c
	return nil
}

// leaderFor builds the manager the running leader would hold.
func leaderFor(
	w *licenseWorld, key string, kube kubernetes.Interface,
) *license.Manager {
	return leaderAt(w, w.srv, key, kube)
}

// leaderAt is leaderFor talking to srv instead of the world's own server.
func leaderAt(
	w *licenseWorld, srv *httptest.Server, key string,
	kube kubernetes.Interface,
) *license.Manager {
	store := kubeInstanceStore{client: kube, namespace: "kwatch"}
	return license.NewManager(license.Config{
		Key: key, Verifier: w.verifier(),
		Client:        &license.Client{Endpoint: srv.URL, HTTP: srv.Client()},
		InstanceStore: store, LeaseStore: &memLease{},
		KubeSystemUID:   kubeSystemUID(kube),
		ClusterUUIDFrom: clusterIDFromUID,
		MoveRequestedAt: store.MoveRequestedAt, MoveFinished: store.FinishMove,
		Now: w.clock.Now, Version: "v1",
	})
}

// answeredBy returns a poll hook that lets the leader handle the request.
func answeredBy(m *license.Manager) func(context.Context, time.Duration) error {
	return func(ctx context.Context, _ time.Duration) error {
		m.CheckMove(ctx)
		return nil
	}
}

func moveFixture(t *testing.T) (*licenseWorld, string, *fake.Clientset) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 30*24*time.Hour)
	kube := fake.NewSimpleClientset(kubeSystemNS(), emptyInstanceSecret("kwatch"))
	return w, key, kube
}

func TestLicenseMoveCLITakesTheLicenseThroughTheLeader(t *testing.T) {
	w, key, kube := moveFixture(t)
	leader := leaderFor(w, key, kube)
	cli := testCLI(w, key, kube)
	cli.sleep = answeredBy(leader)
	var out, errOut bytes.Buffer

	code := cli.move(context.Background(), &out, &errOut)

	require.Zero(t, code, errOut.String())
	require.Equal(t, 1, w.count("/license/move"))
	require.Contains(t, out.String(), "is now active on this cluster")
	require.Contains(t, out.String(), "daily check")
	require.NotContains(t, out.String()+errOut.String(), key)
	require.Equal(t, license.StateActive, leader.Status().State)

	secret, err := kube.CoreV1().Secrets("kwatch").Get(
		context.Background(), licenseInstanceSecret, metav1.GetOptions{})
	require.NoError(t, err)
	require.Empty(t, secret.Data[licenseMoveRequestField],
		"the request is removed once answered")
	require.NotEmpty(t, secret.Data[licenseInstanceKeyField],
		"the instance key exists")

	// Run again: the license is already here.
	out.Reset()
	cli = testCLI(w, key, kube)
	cli.sleep = answeredBy(leader)
	w.clock.Advance(time.Second)
	require.Zero(t, cli.move(context.Background(), &out, &errOut))
	require.Contains(t, out.String(), "already active on this cluster")
}

func TestLicenseMoveCLIClearsAnOlderRelease(t *testing.T) {
	w, key, kube := moveFixture(t)
	store := kubeInstanceStore{client: kube, namespace: "kwatch"}
	require.NoError(t, store.MarkDeactivated(context.Background(), w.clock.Now()))
	cli := testCLI(w, key, kube)
	cli.sleep = answeredBy(leaderFor(w, key, kube))
	var out, errOut bytes.Buffer

	w.clock.Advance(time.Minute)
	require.Zero(t, cli.move(context.Background(), &out, &errOut), errOut.String())

	at, err := store.DeactivatedAt(context.Background())
	require.NoError(t, err)
	require.True(t, at.IsZero(), "a later session must not read it as current")
}

func TestLicenseMoveCLIReportsWhatTheServerSaid(t *testing.T) {
	w, key, kube := moveFixture(t)
	for _, tt := range []struct {
		name, code, want string
		status           int
	}{
		{"limit", "transfer_limit", "A slot frees up in 1 day.", 429},
		{"invalid key", "invalid_key", "does not accept this key", 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			refuse := httptest.NewServer(http.HandlerFunc(func(
				rw http.ResponseWriter, _ *http.Request,
			) {
				rw.WriteHeader(tt.status)
				_, _ = rw.Write([]byte(`{"error":"` + tt.code +
					`","retry_after":86400}`))
			}))
			defer refuse.Close()
			cli := testCLI(w, key, kube)
			cli.sleep = answeredBy(leaderAt(w, refuse, key, kube))
			var out, errOut bytes.Buffer

			code := cli.move(context.Background(), &out, &errOut)

			require.NotZero(t, code)
			require.Contains(t, errOut.String(), tt.want)
			w.clock.Advance(time.Second)
		})
	}
}

func TestLicenseMoveCLITellsHowManyDaysUntilASlotFrees(t *testing.T) {
	require.Contains(t, slotText(72*time.Hour), "in 3 days")
	require.Contains(t, slotText(25*time.Hour), "in 2 days")
	require.Contains(t, slotText(time.Hour), "in 1 day")
	require.Contains(t, slotText(0), "ages past 30 days")
}

func TestLicenseMoveCLIGivesUpWithoutALeader(t *testing.T) {
	w, key, kube := moveFixture(t)
	cli := testCLI(w, key, kube)
	cli.sleep = func(context.Context, time.Duration) error {
		return context.DeadlineExceeded
	}
	var out, errOut bytes.Buffer

	code := cli.move(context.Background(), &out, &errOut)

	require.NotZero(t, code)
	require.Contains(t, errOut.String(), "no answer from the running kwatch")
	require.Zero(t, w.total(), "the command itself never calls the server")
}

func TestLicenseMoveCLIFailsClearly(t *testing.T) {
	w, key, kube := moveFixture(t)
	var out, errOut bytes.Buffer
	require.NotZero(t, testCLI(w, "", kube).move(
		context.Background(), &out, &errOut))
	require.Contains(t, errOut.String(), "no license key")

	errOut.Reset()
	require.NotZero(t, testCLI(w, "nonsense", kube).move(
		context.Background(), &out, &errOut))
	require.NotContains(t, errOut.String(), "nonsense")

	errOut.Reset()
	cli := testCLI(w, key, kube)
	cli.kube = func() (kubernetes.Interface, error) {
		return nil, context.DeadlineExceeded
	}
	require.NotZero(t, cli.move(context.Background(), &out, &errOut))
	require.Contains(t, errOut.String(), "inside the kwatch pod")
}
