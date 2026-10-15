package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type memLeaseStore struct {
	mu    sync.Mutex
	lease string
	clock ClockGuard
	saves int
}

func (s *memLeaseStore) LoadLease() (string, ClockGuard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lease, s.clock, nil
}

func (s *memLeaseStore) SaveLease(l string, c ClockGuard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lease, s.clock = l, c
	s.saves++
	return nil
}

type memInstanceStore struct {
	mu  sync.Mutex
	key ed25519.PrivateKey
}

func (s *memInstanceStore) Load(
	context.Context,
) (ed25519.PrivateKey, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key, s.key != nil, nil
}

func (s *memInstanceStore) Save(_ context.Context, k ed25519.PrivateKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = k
	return nil
}

// fakeWorker mimics the license Worker closely enough for the client.
type fakeWorker struct {
	t     *testing.T
	clock *fakeClock
	priv  ed25519.PrivateKey
	srv   *httptest.Server

	mu       sync.Mutex
	reqs     map[string]int
	bound    bool
	cluster  string
	proof    string
	instance string
	override func(path string) (
		status int, body map[string]any, hdr map[string]string,
	)
	badSigs    int
	lastActive map[string]any
	lastRenew  map[string]any
}

func newFakeWorker(
	t *testing.T, clock *fakeClock, priv ed25519.PrivateKey,
) *fakeWorker {
	w := &fakeWorker{t: t, clock: clock, priv: priv, reqs: map[string]int{}}
	w.srv = httptest.NewServer(http.HandlerFunc(w.serve))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *fakeWorker) count(path string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reqs[path]
}

func (w *fakeWorker) total() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.reqs {
		n += c
	}
	return n
}

func (w *fakeWorker) set(
	f func(path string) (int, map[string]any, map[string]string),
) {
	w.mu.Lock()
	w.override = f
	w.mu.Unlock()
}

func (w *fakeWorker) lease(lid string) string {
	now := w.clock.Now()
	return SignLease(Lease{KID: "v1", LID: lid, ClusterUUID: w.cluster,
		ClusterProof: w.proof, InstancePub: w.instance, Plan: PlanPro,
		LicExp: now.Add(300 * 24 * time.Hour).Unix(),
		Iat:    now.Unix(), Nbf: now.Add(-time.Hour).Unix(),
		Exp:     now.Add(3 * 24 * time.Hour).Unix(),
		SrvTime: now.Unix()}, w.priv)
}

func (w *fakeWorker) serve(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reqs[r.URL.Path]++
	reply := func(status int, body map[string]any, hdr map[string]string) {
		body["srv_time"] = w.clock.Now().Unix()
		for k, v := range hdr {
			rw.Header().Set(k, v)
		}
		rw.WriteHeader(status)
		_ = json.NewEncoder(rw).Encode(body)
	}
	if w.override != nil {
		if st, body, hdr := w.override(r.URL.Path); st != 0 {
			reply(st, body, hdr)
			return
		}
	}
	var b map[string]any
	_ = json.Unmarshal(raw, &b)
	str := func(k string) string { s, _ := b[k].(string); return s }
	switch r.URL.Path {
	case "/license/activate", "/license/move":
		w.lastActive = b
		if str("nonce") == "" || str("kwatch_version") == "" {
			reply(400, map[string]any{"error": "invalid_request"}, nil)
			return
		}
		k, err := ParseKey(str("key"))
		if err != nil {
			reply(400, map[string]any{"error": "invalid_key"}, nil)
			return
		}
		if w.bound && w.cluster != str("cluster_uuid") &&
			r.URL.Path == "/license/activate" {
			reply(409, map[string]any{"error": "bound_to_other_cluster",
				"bound_since": 1000}, nil)
			return
		}
		w.bound, w.cluster, w.proof, w.instance = true, str("cluster_uuid"),
			str("cluster_proof"), str("instance_pub")
		reply(200, map[string]any{"lease": w.lease(k.LID)}, nil)
	case "/license/renew", "/license/deactivate":
		if !w.bound {
			reply(401, map[string]any{"error": "bad_signature"}, nil)
			return
		}
		pub, _ := base64.RawURLEncoding.DecodeString(w.instance)
		sig, err := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Kwatch-Sig"))
		if err != nil || !ed25519.Verify(pub, raw, sig) {
			w.badSigs++
			reply(401, map[string]any{"error": "bad_signature"}, nil)
			return
		}
		if ts, _ := b["ts"].(float64); int64(ts) != w.clock.Now().Unix() {
			reply(400, map[string]any{"error": "stale_timestamp"}, nil)
			return
		}
		if r.URL.Path == "/license/deactivate" {
			w.bound = false
			reply(200, map[string]any{"ok": true}, nil)
			return
		}
		w.lastRenew = b
		reply(200, map[string]any{"lease": w.lease(str("lid"))}, nil)
	default:
		reply(404, map[string]any{"error": "not_found"}, nil)
	}
}

type harness struct {
	t      *testing.T
	clock  *fakeClock
	worker *fakeWorker
	leases *memLeaseStore
	inst   *memInstanceStore
	uid    string
	uidMu  sync.Mutex
	key    string
	sleeps []time.Duration
	m      *Manager
}

func newHarness(t *testing.T) *harness {
	v, priv := testEnv()
	_ = v
	h := &harness{
		t: t, clock: &fakeClock{t: testNow}, leases: &memLeaseStore{},
		inst: &memInstanceStore{}, uid: "kube-uid",
	}
	h.key = SignKey(testKey(testNow.Add(365*24*time.Hour)), priv)
	h.worker = newFakeWorker(t, h.clock, priv)
	h.m = h.newManager(h.key)
	return h
}

func (h *harness) setUID(s string) {
	h.uidMu.Lock()
	h.uid = s
	h.uidMu.Unlock()
}

func (h *harness) newManager(key string) *Manager {
	v, _ := testEnv()
	return NewManager(Config{
		Key: key, Verifier: v,
		Client: &Client{
			Endpoint: h.worker.srv.URL, HTTP: h.worker.srv.Client(),
		},
		InstanceStore: h.inst, LeaseStore: h.leases, ClusterUUID: "uuid-1",
		KubeSystemUID: func(context.Context) (string, error) {
			h.uidMu.Lock()
			defer h.uidMu.Unlock()
			return h.uid, nil
		},
		Version:   "v1.2.3",
		NodeCount: func(context.Context) int { return 3 },
		Now:       h.clock.Now,
		Sleep: func(ctx context.Context, d time.Duration) error {
			h.sleeps = append(h.sleeps, d)
			return ctx.Err()
		},
		Jitter: func(time.Duration) time.Duration { return 0 },
	})
}

// run initializes the manager and performs one cycle.
func (h *harness) run() time.Duration {
	ctx := context.Background()
	h.m.init(ctx)
	return h.m.cycle(ctx)
}

func TestManagerNoKeyNoNetwork(t *testing.T) {
	h := newHarness(t)
	m := h.newManager("")
	m.Start(context.Background())
	m.Wait()
	if st := m.Status(); st.State != StateCommunity {
		t.Fatalf("state = %v", st.State)
	}
	if err := m.Deactivate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := h.worker.total(); n != 0 {
		t.Fatalf("requests = %d, want 0", n)
	}
	if h.leases.saves != 0 || h.inst.key != nil {
		t.Fatal("community mode must not touch stores")
	}
}

func TestManagerActivate(t *testing.T) {
	h := newHarness(t)
	ch := h.m.Subscribe()
	d := h.run()
	st := h.m.Status()
	if st.State != StateActive || st.Plan != PlanPro {
		t.Fatalf("status = %+v", st)
	}
	if d != 24*time.Hour {
		t.Fatalf("next delay = %v", d)
	}
	if h.leases.lease == "" || h.leases.clock.MaxSeen.IsZero() {
		t.Fatal("lease and clock not persisted")
	}
	a := h.worker.lastActive
	if a["kwatch_version"] != "v1.2.3" || a["node_count"].(float64) != 3 ||
		a["cluster_uuid"] != "uuid-1" {
		t.Fatalf("activate body = %v", a)
	}
	if a["cluster_proof"] != ClusterProof("kube-uid", "lic_abcdef1234") {
		t.Fatal("proof not derived from live uid")
	}
	// pending then active, nothing more.
	var got []State
	for len(ch) > 0 {
		got = append(got, (<-ch).State)
	}
	if len(got) == 0 || got[len(got)-1] != StateActive {
		t.Fatalf("emitted %v", got)
	}
}

func TestManagerInitialSyncActivatesBeforeStartReturns(t *testing.T) {
	h := newHarness(t)
	m := NewManager(Config{
		Key: h.key, Verifier: h.m.cfg.Verifier, Client: h.m.cfg.Client,
		InstanceStore: h.inst, LeaseStore: h.leases,
		ClusterUUID: "uuid-1", KubeSystemUID: h.m.cfg.KubeSystemUID,
		Version: "v1.2.3", Now: h.clock.Now, Sleep: h.m.cfg.Sleep,
		Jitter:             func(time.Duration) time.Duration { return 0 },
		InitialSyncTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	// No waiting: the first activation already happened.
	if st := m.Status(); st.State != StateActive {
		t.Fatalf("state = %v, want active from Start", st.State)
	}
	if n := h.worker.count("/license/activate"); n != 1 {
		t.Fatalf("activations = %d, want 1", n)
	}
	cancel()
	m.Wait()
	if n := h.worker.count("/license/activate"); n != 1 {
		t.Fatalf("the loop repeated the activation: %d", n)
	}
}

func TestManagerInitialSyncGivesUpAtItsTimeout(t *testing.T) {
	h := newHarness(t)
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		time.Sleep(300 * time.Millisecond)
		return 500, map[string]any{}, nil
	})
	m := NewManager(Config{
		Key: h.key, Verifier: h.m.cfg.Verifier,
		Client: &Client{
			Endpoint: h.worker.srv.URL, HTTP: h.worker.srv.Client(),
		},
		InstanceStore: h.inst, LeaseStore: h.leases,
		ClusterUUID: "uuid-1", KubeSystemUID: h.m.cfg.KubeSystemUID,
		Now: h.clock.Now, Sleep: h.m.cfg.Sleep,
		Jitter:             func(time.Duration) time.Duration { return 0 },
		InitialSyncTimeout: 50 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	m.Start(ctx)
	if time.Since(start) > 250*time.Millisecond {
		t.Fatalf("Start blocked past its timeout: %v", time.Since(start))
	}
	if st := m.Status(); st.State.ProEnabled() {
		t.Fatalf("state = %v without a lease", st.State)
	}
	cancel()
	m.Wait()
}
