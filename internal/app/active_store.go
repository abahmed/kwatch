package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/storage"
)

// openStore opens the state file and claims it for this leadership term,
// so a deposed leader can no longer write. The claim is made only after
// the Lease is confirmed to name this replica; the epoch itself is the
// store's own counter, so a recreated Lease cannot fence the new leader.
// The file is opened with DeferRepair: deleting an unreadable file or
// rewriting an oversized one waits for Claim, so a replica that turns out
// not to hold the Lease never changes the file.
func openStore(ctx context.Context, deps *serverDeps) (*storage.Store, error) {
	identity, err := podIdentity()
	if err != nil {
		return nil, err
	}
	limit, err := volumeLimit()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir(), "state.db")
	steps := newStartupSteps(deps.clients.Clock.Now)
	s, err := storage.Open(path, storage.Options{
		Now:         deps.clients.Clock.Now,
		DeferRepair: true,
		VolumeLimit: limit,
	})
	steps.done("open")
	if err != nil {
		return nil, err
	}
	err = verifyLeaseHolder(ctx, deps, identity)
	steps.done("lease check")
	if err != nil {
		closeStore(s)
		return nil, err
	}
	epoch, err := s.Claim()
	steps.done("claim")
	if err != nil {
		closeStore(s)
		return nil, err
	}
	size := int64(0)
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	klog.InfoS("claimed state store", "component", "state",
		"operation", "claim", "epoch", epoch, "fileBytes", size,
		"schemaVersion", storage.SchemaVersion,
		"openMs", steps.took["open"].Milliseconds(),
		"leaseCheckMs", steps.took["lease check"].Milliseconds(),
		"claimMs", steps.took["claim"].Milliseconds(),
		"totalMs", steps.total().Milliseconds())
	return s, nil
}

// startupSteps times the steps of openStore, so the gap between the
// Lease and the claim is accounted for. A step that takes longer than
// storage.SlowStep is logged as soon as it ends.
type startupSteps struct {
	now   func() time.Time
	began time.Time
	last  time.Time
	took  map[string]time.Duration
}

func newStartupSteps(now func() time.Time) *startupSteps {
	at := now()
	return &startupSteps{
		now: now, began: at, last: at, took: map[string]time.Duration{},
	}
}

// done ends the step that started when the previous one ended.
func (s *startupSteps) done(name string) {
	at := s.now()
	s.took[name] = at.Sub(s.last)
	storage.LogIfSlow(name, s.took[name])
	s.last = at
}

func (s *startupSteps) total() time.Duration {
	return s.last.Sub(s.began)
}

// reportStoreReset makes a reset at open visible on /health. The state
// stays usable and ready; the reason is kept for the whole session so an
// operator can see that history was discarded.
func reportStoreReset(deps *serverDeps, s *storage.Store) {
	reset, ok := s.Reset()
	if !ok {
		return
	}
	klog.InfoS("state store started fresh", "component", "state",
		"operation", "reset", "reason", reset.Reason,
		"oldVersion", reset.OldVersion)
	recordStoreReset(metrics.DefaultRegistry(), reset)
	if deps.healthServer != nil {
		deps.healthServer.SetComponentStatus(
			"state-store", "degraded", "storage_reset", true)
	}
}

// runCompactor enforces storage retention and size caps every
// storage.CompactInterval. It runs as its own component, never on the
// decision loop; each batch is one short fenced transaction, so it stops
// within one batch of cancellation and before the store is closed.
// published is shared by every restart of the component: the store's
// counters are lifetime totals, so fresh metrics after a restart would
// count them twice.
func runCompactor(
	s *storage.Store, published *storageMetrics,
) func(context.Context) error {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(storage.CompactInterval)
		defer ticker.Stop()
		compactor := storage.NewCompactor(s, storage.DefaultPolicy())
		publishCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			published.publishUntil(publishCtx, s.Stats, compactor.Passes())
		}()
		err := compactor.Run(ctx, ticker.C)
		stop()
		<-done
		return err
	}
}

// storeMetrics builds the storage metrics for one store. It is made once
// per session and shared by every restart of the compactor.
func storeMetrics(deps *serverDeps) *storageMetrics {
	published := newStorageMetrics(metrics.DefaultRegistry())
	published.overCap = reportOverCap(deps)
	return published
}

func closeStore(s *storage.Store) {
	if err := s.Close(); err != nil {
		klog.ErrorS(err, "close state store", "component", "state")
	}
}

// errNotLeaseHolder means the Lease names another replica.
var errNotLeaseHolder = errors.New("lease is held by another replica")

// verifyLeaseHolder confirms that the leader Lease names identity.
func verifyLeaseHolder(
	ctx context.Context, deps *serverDeps, identity string,
) error {
	lease, err := electionClient(deps.clients).CoordinationV1().
		Leases(kubeclient.GetNamespace()).Get(ctx, electionLeaseName(),
		metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read leader lease: %w", err)
	}
	holder := ""
	if lease.Spec.HolderIdentity != nil {
		holder = *lease.Spec.HolderIdentity
	}
	if holder != identity {
		return errNotLeaseHolder
	}
	return nil
}

// lastRenewalFrom reads the last successful Lease write from health.
func lastRenewalFrom(deps *serverDeps) func() time.Time {
	return func() time.Time {
		status := deps.healthServer.LeadershipStatus()
		if status == nil {
			return time.Time{}
		}
		return status.LastRenewal
	}
}
