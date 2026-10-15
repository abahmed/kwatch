package license

import (
	"context"
	"crypto/ed25519"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	renewInterval = 24 * time.Hour
	renewJitter   = time.Hour
	// clockPersistEvery is how far the clock guard may advance in memory
	// before it is written to the LeaseStore again.
	clockPersistEvery = time.Hour
	uidTimeout        = 5 * time.Second
)

var retryBackoff = [...]time.Duration{
	time.Minute, 5 * time.Minute, 15 * time.Minute,
	30 * time.Minute, time.Hour,
}

// LeaseStore persists the current lease and the clock guard.
type LeaseStore interface {
	LoadLease() (lease string, clock ClockGuard, err error)
	SaveLease(lease string, clock ClockGuard) error
}

// Config configures a Manager.
type Config struct {
	// Key is the license key; empty means community mode with no network use.
	Key string
	// Verifier defaults to DefaultVerifier.
	Verifier      *Verifier
	Client        *Client
	InstanceStore InstanceKeyStore
	LeaseStore    LeaseStore
	// ClusterUUID is a fixed cluster id, used when ClusterUUIDFrom is nil.
	ClusterUUID string
	// ClusterUUIDFrom derives the cluster id from the live kube-system UID.
	// Licensing never falls back to another id: when the UID cannot be read
	// the license stays pending and the read is retried with backoff.
	ClusterUUIDFrom func(uid string) string
	// DeactivatedAt, when set, reports when another process (the
	// `kwatch license deactivate` command) released this cluster; zero when
	// it never did. The manager then drops a lease issued before that
	// moment and stops contacting the server for this process lifetime.
	DeactivatedAt func(ctx context.Context) (time.Time, error)
	// ProcessStart separates a release made while this process ran (stay
	// released) from an older one (a restart activates again). Zero treats
	// every release as current.
	ProcessStart time.Time
	// KubeSystemUID is read live for every proof; it is never cached into one.
	KubeSystemUID func(ctx context.Context) (string, error)
	Version       string
	NodeCount     func(ctx context.Context) int
	// InitialSyncTimeout, when positive, makes Start run the first
	// activation or renewal synchronously (bounded by it) when one is due,
	// so a valid license is Active when Start returns. Zero keeps Start
	// non-blocking.
	InitialSyncTimeout time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
	// Sleep waits for d or until ctx is done (returning ctx.Err()). Defaults
	// to a timer; tests inject a fake.
	Sleep func(ctx context.Context, d time.Duration) error
	// Jitter returns an offset in [-max, +max] added to the renew interval.
	// Defaults to a uniform random offset.
	Jitter func(max time.Duration) time.Duration
}

// Manager owns activation, renewal and the current licensing status.
type Manager struct {
	cfg Config
	v   *Verifier
	lid string // from the key; empty if the key does not parse

	opMu sync.Mutex // serializes network operations

	mu       sync.Mutex
	lease    string
	clock    ClockGuard
	verdict  Verdict
	priv     ed25519.PrivateKey
	pub      string
	lastUID  string // fallback for Status only when the live read fails
	released bool
	last     Status
	haveLast bool
	subs     []chan Status
	failures int
	anchor   time.Time // first local reading, for the monotonic cross-check
	savedMax time.Time // MaxSeen as last written to the LeaseStore

	done chan struct{}
}

// NewManager returns a Manager. Nothing runs until Start.
func NewManager(cfg Config) *Manager {
	if cfg.Verifier == nil {
		cfg.Verifier = DefaultVerifier()
	}
	if cfg.Client == nil {
		cfg.Client = &Client{}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepCtx
	}
	if cfg.Jitter == nil {
		cfg.Jitter = func(max time.Duration) time.Duration {
			return time.Duration(rand.Int64N(int64(2*max)+1)) - max
		}
	}
	m := &Manager{cfg: cfg, v: cfg.Verifier, done: make(chan struct{})}
	if k, err := ParseKey(cfg.Key); err == nil {
		m.lid = k.LID
	}
	return m
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Start loads cached state synchronously and, when a key is configured,
// launches the activation and renewal loop; it returns immediately. With an
// empty key the state is community and no network call is ever made.
func (m *Manager) Start(ctx context.Context) {
	if m.cfg.Key == "" {
		m.refresh(ctx)
		close(m.done)
		return
	}
	m.init(ctx)
	if m.cfg.InitialSyncTimeout > 0 {
		delay := m.initialDelay(ctx)
		if delay == 0 {
			syncCtx, cancel := context.WithTimeout(
				ctx, m.cfg.InitialSyncTimeout)
			delay = m.cycle(syncCtx)
			cancel()
		}
		go func() {
			defer close(m.done)
			m.loopFrom(ctx, delay)
		}()
		return
	}
	go func() {
		defer close(m.done)
		m.loop(ctx)
	}()
}

// Wait blocks until the background loop has stopped.
func (m *Manager) Wait() { <-m.done }

func (m *Manager) init(ctx context.Context) {
	if m.cfg.LeaseStore != nil {
		lease, clock, err := m.cfg.LeaseStore.LoadLease()
		if err != nil {
			klog.ErrorS(err, "load cached license lease", "component", "license")
		} else {
			m.mu.Lock()
			m.lease, m.clock, m.savedMax = lease, clock, clock.MaxSeen
			m.mu.Unlock()
		}
	}
	m.applyDeactivation(ctx)
	_ = m.ensureInstance(ctx) // logged; cycle retries
	m.refresh(ctx)
}

func (m *Manager) ensureInstance(ctx context.Context) error {
	m.mu.Lock()
	have := m.priv != nil
	m.mu.Unlock()
	if have {
		return nil
	}
	if m.cfg.InstanceStore == nil {
		return errors.New("license: no instance key store")
	}
	priv, err := EnsureInstanceKey(ctx, m.cfg.InstanceStore)
	if err != nil {
		klog.ErrorS(err, "instance key unavailable", "component", "license")
		return err
	}
	m.mu.Lock()
	m.priv = priv
	m.pub = PublicString(priv.Public().(ed25519.PublicKey))
	m.mu.Unlock()
	return nil
}

func (m *Manager) liveUID(ctx context.Context) (string, error) {
	if m.cfg.KubeSystemUID == nil {
		return "", errors.New("license: no kube-system uid source")
	}
	ctx, cancel := context.WithTimeout(ctx, uidTimeout)
	defer cancel()
	return m.cfg.KubeSystemUID(ctx)
}

// Status returns the current status, recomputed with the live clock and a
// live kube-system UID read (a failed read falls back to the last good one).
func (m *Manager) Status() Status { return m.refresh(context.Background()) }

// refresh recomputes the status and notifies subscribers if State or Plan
// changed.
func (m *Manager) refresh(ctx context.Context) Status {
	if m.cfg.Key == "" {
		st := Status{State: StateCommunity}
		m.publish(st)
		return st
	}
	uid, err := m.liveUID(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		uid = m.lastUID
	} else {
		m.lastUID = uid
	}
	now := m.cfg.Now()
	m.observeLocalLocked(now)
	if uid == "" {
		// No cluster identity yet: never guess one. Pending until the
		// kube-system UID can be read.
		st := Status{State: StatePending, Plan: m.last.Plan, LID: MaskLID(m.lid),
			Reason: "cluster identity unavailable: " + errText(err)}
		m.publishLocked(st)
		return st
	}
	st := Evaluate(Inputs{
		Verifier:  m.v,
		KeyString: m.cfg.Key,
		Lease:     m.lease,
		Binding: Binding{
			LID: m.lid, ClusterUUID: m.clusterUUID(uid),
			ClusterProof: ClusterProof(uid, m.lid), InstancePub: m.pub,
		},
		Now: now, Clock: m.clock, ServerVerdict: m.verdict,
	})
	m.publishLocked(st)
	return st
}

func (m *Manager) publish(st Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishLocked(st)
}

func (m *Manager) publishLocked(st Status) {
	if m.haveLast && m.last.State == st.State && m.last.Plan == st.Plan {
		m.last = st
		return
	}
	m.last, m.haveLast = st, true
	for _, ch := range m.subs {
		select {
		case ch <- st:
		default:
			select { // coalesce: drop the stale value
			case <-ch:
			default:
			}
			select {
			case ch <- st:
			default:
			}
		}
	}
}

// Subscribe returns a channel that receives the status whenever State or Plan
// changes. It holds at most one pending value; a slow reader sees the latest.
func (m *Manager) Subscribe() <-chan Status {
	ch := make(chan Status, 1)
	m.mu.Lock()
	m.subs = append(m.subs, ch)
	m.mu.Unlock()
	return ch
}

func (m *Manager) effective() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.cfg.Now()
	m.observeLocalLocked(now)
	return m.clock.Effective(now)
}

func (m *Manager) clusterUUID(uid string) string {
	if m.cfg.ClusterUUIDFrom != nil {
		return m.cfg.ClusterUUIDFrom(uid)
	}
	return m.cfg.ClusterUUID
}

func errText(err error) string {
	if err == nil {
		return "unknown"
	}
	return err.Error()
}

func (m *Manager) interval() time.Duration {
	return renewInterval + m.cfg.Jitter(renewJitter)
}

func (m *Manager) backoff() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := min(m.failures, len(retryBackoff)-1)
	m.failures++
	return retryBackoff[i]
}
