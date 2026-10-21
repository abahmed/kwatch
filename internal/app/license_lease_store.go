package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/license"
)

const (
	// licenseLeaseCacheField mirrors the lease and the clock guard in the
	// instance Secret, so Pro offline grace survives an emptyDir restart.
	// Both are kept in one field: deleting it removes the lease too.
	licenseLeaseCacheField = "lease-cache"
	// leaseMirrorEvery is how far the clock guard may advance before the
	// Secret copy is written again; the lease is written on every change.
	leaseMirrorEvery   = time.Hour
	leaseMirrorTimeout = 5 * time.Second
)

// leaseRecord keeps the lease and the clock guard as one value, so the
// guard cannot be deleted on its own to make an old lease usable again.
type leaseRecord struct {
	Lease   string    `json:"lease"`
	MaxSeen time.Time `json:"max_seen"`
}

// boltLeaseStore keeps the lease and the clock guard in the state file.
type boltLeaseStore struct{ disk diskState }

// LoadLease implements license.LeaseStore. It also reads the older
// layout: the lease as a bare string with the guard under its own key.
func (s boltLeaseStore) LoadLease() (string, license.ClockGuard, error) {
	var raw json.RawMessage
	var rec leaseRecord
	found, err := s.disk.get(stateLicenseLease, &raw)
	if err != nil {
		return "", license.ClockGuard{}, err
	}
	if found && json.Unmarshal(raw, &rec) != nil {
		var legacy string
		if json.Unmarshal(raw, &legacy) == nil {
			rec.Lease = legacy
		}
	}
	var old license.ClockGuard
	if _, err := s.disk.get(stateLicenseClock, &old); err != nil {
		return "", license.ClockGuard{}, err
	}
	clock := license.ClockGuard{MaxSeen: rec.MaxSeen}
	clock.Observe(old.MaxSeen)
	return rec.Lease, clock, nil
}

// SaveLease implements license.LeaseStore.
func (s boltLeaseStore) SaveLease(
	lease string, clock license.ClockGuard,
) error {
	return s.disk.put(stateLicenseLease,
		leaseRecord{Lease: lease, MaxSeen: clock.MaxSeen})
}

// LoadLeaseCache reads the Secret copy of the lease; empty when absent.
func (s kubeInstanceStore) LoadLeaseCache(
	ctx context.Context,
) (leaseRecord, error) {
	var rec leaseRecord
	secret, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return rec, err
	}
	raw := secret.Data[licenseLeaseCacheField]
	if len(raw) == 0 || json.Unmarshal(raw, &rec) != nil {
		return leaseRecord{}, nil
	}
	return rec, nil
}

// SaveLeaseCache writes the Secret copy of the lease. The lease is signed
// and bound to this cluster and instance, so it is safe to keep there.
func (s kubeInstanceStore) SaveLeaseCache(
	ctx context.Context, rec leaseRecord,
) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	existing, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[licenseLeaseCacheField] = raw
	_, err = s.secrets().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// mirroredLeaseStore keeps the lease in the state file and a copy in the
// instance Secret. Loading takes the newer verified lease and the later
// guard of the two, so a lost data volume (emptyDir) keeps offline grace.
type mirroredLeaseStore struct {
	local    boltLeaseStore
	secret   kubeInstanceStore
	verifier *license.Verifier

	mu   sync.Mutex
	last leaseRecord // what the Secret is known to hold
}

func newMirroredLeaseStore(
	disk diskState, kube kubernetes.Interface, v *license.Verifier,
) *mirroredLeaseStore {
	return &mirroredLeaseStore{
		local: boltLeaseStore{disk: disk},
		secret: kubeInstanceStore{
			client: kube, namespace: kubeclient.GetNamespace(),
		},
		verifier: v,
	}
}

// LoadLease implements license.LeaseStore.
func (s *mirroredLeaseStore) LoadLease() (
	string, license.ClockGuard, error,
) {
	lease, clock, localErr := s.local.LoadLease()
	ctx, cancel := context.WithTimeout(
		context.Background(), leaseMirrorTimeout)
	defer cancel()
	rec, err := s.secret.LoadLeaseCache(ctx)
	if err != nil {
		klog.V(2).InfoS("read license lease copy failed",
			"component", "license", "err", err.Error())
		return lease, clock, localErr
	}
	s.mu.Lock()
	s.last = rec
	s.mu.Unlock()
	if localErr != nil {
		lease, clock = "", license.ClockGuard{}
	}
	clock.Observe(rec.MaxSeen)
	return license.NewerLease(s.verifier, lease, rec.Lease), clock, nil
}

// SaveLease implements license.LeaseStore. A failed Secret write is
// logged and retried on the next save; only the state file's error is
// returned.
func (s *mirroredLeaseStore) SaveLease(
	lease string, clock license.ClockGuard,
) error {
	localErr := s.local.SaveLease(lease, clock)
	rec := leaseRecord{Lease: lease, MaxSeen: clock.MaxSeen}
	s.mu.Lock()
	defer s.mu.Unlock()
	moved := rec.MaxSeen.Sub(s.last.MaxSeen).Abs()
	if rec.Lease == s.last.Lease && moved < leaseMirrorEvery {
		return localErr
	}
	ctx, cancel := context.WithTimeout(
		context.Background(), leaseMirrorTimeout)
	defer cancel()
	if err := s.secret.SaveLeaseCache(ctx, rec); err != nil {
		klog.ErrorS(err, "mirror license lease to its Secret",
			"component", "license")
		return localErr
	}
	s.last = rec
	return localErr
}
