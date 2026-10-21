package app

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/metrics"
)

const testLID = "lic_testlicense1234"

// testClock is a clock the test moves.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// licenseWorld is a fake license server with the keys it signs under.
type licenseWorld struct {
	t     *testing.T
	clock *testClock
	pub   ed25519.PublicKey
	priv  ed25519.PrivateKey
	srv   *httptest.Server

	mu       sync.Mutex
	calls    map[string]int
	instance string
	ttl      time.Duration
	reject   string // error code to answer with, status 403
}

func newLicenseWorld(t *testing.T) *licenseWorld {
	t.Helper()
	seed := sha256.Sum256([]byte("kwatch-app-license-test"))
	priv := ed25519.NewKeyFromSeed(seed[:])
	w := &licenseWorld{
		t: t, clock: &testClock{t: time.Now().Truncate(time.Second)},
		pub: priv.Public().(ed25519.PublicKey), priv: priv,
		calls: map[string]int{}, ttl: 3 * 24 * time.Hour,
	}
	w.srv = httptest.NewServer(http.HandlerFunc(w.serve))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *licenseWorld) verifier() *license.Verifier {
	return license.NewVerifier(map[string]ed25519.PublicKey{"v1": w.pub})
}

func (w *licenseWorld) key(plan string, expiresIn time.Duration) string {
	now := w.clock.Now()
	return license.SignKey(license.Key{
		V: 1, KID: "v1", LID: testLID, Acc: "acct", Plan: plan,
		Iat: now.Unix(), Exp: now.Add(expiresIn).Unix(), Max: 5,
	}, w.priv)
}

func (w *licenseWorld) count(path string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls[path]
}

func (w *licenseWorld) total() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.calls {
		n += c
	}
	return n
}

func (w *licenseWorld) serve(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls[r.URL.Path]++
	now := w.clock.Now()
	reply := func(status int, body map[string]any) {
		body["srv_time"] = now.Unix()
		rw.WriteHeader(status)
		_ = json.NewEncoder(rw).Encode(body)
	}
	if w.reject != "" {
		reply(http.StatusForbidden, map[string]any{"error": w.reject})
		return
	}
	var req map[string]any
	_ = json.Unmarshal(raw, &req)
	str := func(k string) string { s, _ := req[k].(string); return s }
	if p := r.URL.Path; p == "/license/activate" || p == "/license/move" {
		w.instance = str("instance_pub")
	}
	if r.URL.Path == "/license/deactivate" {
		reply(http.StatusOK, map[string]any{"ok": true})
		return
	}
	lease := license.SignLease(license.Lease{
		KID: "v1", LID: testLID, ClusterUUID: str("cluster_uuid"),
		ClusterProof: str("cluster_proof"), InstancePub: w.instance,
		Plan: license.PlanPro, LicExp: now.Add(300 * 24 * time.Hour).Unix(),
		Iat: now.Unix(), Nbf: now.Add(-time.Hour).Unix(),
		Exp: now.Add(w.ttl).Unix(), SrvTime: now.Unix(),
	}, w.priv)
	reply(http.StatusOK, map[string]any{"lease": lease})
}

// licensedDeps is a leader-session fixture whose license points at w.
func licensedDeps(
	t *testing.T, w *licenseWorld, key string, e feature.Entitlements,
) *serverDeps {
	t.Helper()
	deps := activeDeps(t)
	useLease(t, deps)
	deps.entitlements = e
	useWorldClients(deps, w)
	_, err := deps.clients.Kubernetes.CoreV1().Namespaces().Create(
		context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system", UID: types.UID("uid-kube-system"),
		}}, metav1.CreateOptions{})
	require.NoError(t, err)
	ns := kubeclient.GetNamespace()
	_, err = deps.clients.Kubernetes.CoreV1().Secrets(ns).Create(
		context.Background(), emptyInstanceSecret(ns), metav1.CreateOptions{})
	require.NoError(t, err)
	deps.license = &licenseSetup{
		key: key, endpoint: w.srv.URL, verifier: w.verifier(),
		now: w.clock.Now, tick: 10 * time.Millisecond,
		debounce: 30 * time.Millisecond, initialSync: 5 * time.Second,
	}
	return deps
}

// emptyInstanceSecret is the Secret the manifests ship: Opaque, no data.
func emptyInstanceSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: licenseInstanceSecret, Namespace: namespace,
	}, Type: corev1.SecretTypeOpaque}
}

func instanceSecret(
	t *testing.T, deps *serverDeps,
) (*corev1.Secret, bool) {
	t.Helper()
	list, err := deps.clients.Kubernetes.CoreV1().Secrets("").
		List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	for i := range list.Items {
		if list.Items[i].Name == licenseInstanceSecret {
			return &list.Items[i], true
		}
	}
	require.Empty(t, list.Items, "no other Secret may be created")
	return nil, false
}

// runSession runs a leader session and returns its result channel and a
// stop function.
func runSession(
	t *testing.T, deps *serverDeps,
) (<-chan error, context.CancelFunc) {
	t.Helper()
	deps.readiness.begin(1, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- runActiveComponents(ctx, deps) }()
	return done, cancel
}

func awaitSession(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatal("leader session did not end")
		return nil
	}
}

func scrapeMetrics(t *testing.T) string {
	t.Helper()
	rr := httptest.NewRecorder()
	metrics.DefaultRegistry().Handler().ServeHTTP(
		rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rr.Body.String()
}

func TestLicenseNoKeyMakesNoCallsAndWritesNoKey(t *testing.T) {
	w := newLicenseWorld(t)
	deps := licensedDeps(t, w, "", feature.CommunityEntitlements())
	done, stop := runSession(t, deps)

	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	stop()
	require.NoError(t, awaitSession(t, done))

	require.Zero(t, w.total(), "Community must not call the server")
	secret, found := instanceSecret(t, deps)
	require.True(t, found)
	require.Empty(t, secret.Data, "Community must not write the Secret")
	require.False(t, deps.editionRestart.Load())
	require.Equal(t, feature.CommunityEdition, deps.edition.Load().Edition())
}

func TestLicenseValidKeyIsProFromTheFirstBuild(t *testing.T) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 300*24*time.Hour)
	deps := licensedDeps(t, w, key, feature.CommunityEntitlements())

	// The process booted as Community. Before any component exists the
	// license activates and asks for a rebuild with the Pro edition.
	done, _ := runSession(t, deps)
	require.ErrorIs(t, awaitSession(t, done), errEditionChanged)

	require.True(t, deps.editionRestart.Load())
	require.Equal(t, feature.ProEdition, deps.edition.Load().Edition())
	require.Equal(t, 1, w.count("/license/activate"))
	secret, found := instanceSecret(t, deps)
	require.True(t, found)
	require.Len(t, secret.Data[licenseInstanceKeyField], ed25519.SeedSize)
	require.False(t, deps.healthServer.Ready(), "no component was built")

	text := scrapeMetrics(t)
	require.Contains(t, text, `kwatch_license_state{state="active"} 1`)
	require.Contains(t, text, `kwatch_license_state{state="community"} 0`)
	require.Contains(t, text, `kwatch_license_renewals_total{result="success"}`)
	require.NotZero(t, metrics.DefaultRegistry().LicenseExpiry.Load())

	// The rebuilt generation is Pro: it keeps the cached lease and runs.
	deps.entitlements = feature.ProEntitlements()
	deps.editionRestart.Store(false)
	done, stop := runSession(t, deps)
	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)
	stop()
	require.NoError(t, awaitSession(t, done))
	require.Equal(t, 1, w.count("/license/activate"),
		"the cached lease is reused")
	require.Zero(t, w.count("/license/renew"))
}

// activate runs the first generation so a lease is cached, then returns
// deps ready for the Pro generation.
func activated(t *testing.T, w *licenseWorld, key string) *serverDeps {
	t.Helper()
	deps := licensedDeps(t, w, key, feature.CommunityEntitlements())
	done, _ := runSession(t, deps)
	require.ErrorIs(t, awaitSession(t, done), errEditionChanged)
	deps.entitlements = feature.ProEntitlements()
	deps.editionRestart.Store(false)
	return deps
}

func TestLicenseGraceDoesNotRestartTheSession(t *testing.T) {
	w := newLicenseWorld(t)
	w.ttl = time.Hour
	deps := activated(t, w, w.key(license.PlanPro, 300*24*time.Hour))
	done, stop := runSession(t, deps)
	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)

	w.clock.Advance(2 * time.Hour) // past the lease, inside the grace
	require.Eventually(t, func() bool {
		return metrics.DefaultRegistry().LicenseStateIndex.Load() == 4
	}, 5*time.Second, 5*time.Millisecond, "state never became grace")
	time.Sleep(300 * time.Millisecond) // many debounce periods
	require.False(t, deps.editionRestart.Load(),
		"active to grace keeps Pro and must not restart")
	require.Contains(t, scrapeMetrics(t),
		`kwatch_license_state{state="grace"} 1`)

	stop()
	require.NoError(t, awaitSession(t, done))
}

func TestLicenseExpiryRestartsTheSessionOnce(t *testing.T) {
	w := newLicenseWorld(t)
	deps := activated(t, w, w.key(license.PlanPro, 300*24*time.Hour))
	done, _ := runSession(t, deps)
	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)

	w.clock.Advance(15 * 24 * time.Hour) // past the lease and its grace

	require.ErrorIs(t, awaitSession(t, done), errEditionChanged)
	require.True(t, deps.editionRestart.Load())
	require.Equal(t, feature.CommunityEdition, deps.edition.Load().Edition())
	require.Contains(t, scrapeMetrics(t),
		`kwatch_license_state{state="expired"} 1`)
	// The channel results exactly once: nothing more arrives.
	select {
	case err := <-done:
		t.Fatalf("second session result: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestLicenseRestartRequestIsIdempotent(t *testing.T) {
	l := &licenseSession{
		deps: &serverDeps{}, restart: make(chan struct{}),
	}
	st := license.Status{State: license.StateExpired}
	l.requestRestart(st)
	l.requestRestart(st)
	l.requestRestart(st)
	select {
	case <-l.restarted():
	default:
		t.Fatal("restart was not requested")
	}
	require.True(t, l.deps.editionRestart.Load())
}

func TestLicenseRevokedKeyStaysCommunity(t *testing.T) {
	w := newLicenseWorld(t)
	w.reject = "revoked"
	deps := licensedDeps(t, w,
		w.key(license.PlanPro, 300*24*time.Hour),
		feature.CommunityEntitlements())
	done, stop := runSession(t, deps)

	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)
	require.False(t, deps.editionRestart.Load())
	require.Contains(t, scrapeMetrics(t),
		`kwatch_license_renewals_total{result="rejected"}`)
	stop()
	require.NoError(t, awaitSession(t, done))
}

func TestLicenseUntrustedKeyIsInvalidWithTheDefaultVerifier(t *testing.T) {
	w := newLicenseWorld(t)
	deps := licensedDeps(t, w,
		w.key(license.PlanPro, 300*24*time.Hour),
		feature.CommunityEntitlements())
	deps.license.verifier = nil // production: no trusted key compiled in
	done, stop := runSession(t, deps)

	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)
	stop()
	require.NoError(t, awaitSession(t, done))
	require.Zero(t, w.total())
	require.Equal(t, feature.CommunityEdition, deps.edition.Load().Edition())
}

func TestNewLicenseSetupReadsTheConfiguration(t *testing.T) {
	cfg := config.DefaultConfig()
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	setup := newLicenseSetup(config.RuntimeConfigFor(cfg), start)
	require.Empty(t, setup.key)
	require.Equal(t, start, setup.processStartTime())
	require.Equal(t, license.DefaultEndpoint, setup.endpoint)
	require.Nil(t, setup.verifier, "production trusts only compiled-in keys")

	cfg.License = config.License{Key: "KW1-x", Endpoint: "http://test"}
	setup = newLicenseSetup(config.RuntimeConfigFor(cfg), start)
	require.Equal(t, "KW1-x", setup.key)
	require.Equal(t, "http://test", setup.endpoint)

	cfg.License.Endpoint = ""
	setup = newLicenseSetup(config.RuntimeConfigFor(cfg), start)
	require.Equal(t, license.DefaultEndpoint, setup.endpoint)
}
