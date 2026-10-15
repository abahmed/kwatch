package license

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/klog/v2"
)

func (m *Manager) loop(ctx context.Context) {
	m.loopFrom(ctx, m.initialDelay(ctx))
}

// loopFrom waits delay, then runs cycles until ctx ends.
func (m *Manager) loopFrom(ctx context.Context, delay time.Duration) {
	for {
		if err := m.cfg.Sleep(ctx, delay); err != nil || ctx.Err() != nil {
			return
		}
		delay = m.cycle(ctx)
	}
}

// initialDelay is zero unless a valid lease is cached, in which case the
// first renewal is due one interval after that lease was issued.
func (m *Manager) initialDelay(ctx context.Context) time.Duration {
	b, err := m.binding(ctx)
	if err != nil {
		return 0
	}
	m.mu.Lock()
	lease := m.lease
	m.mu.Unlock()
	eff := m.effective()
	l, err := m.v.VerifyLease(lease, b, eff)
	if err != nil {
		return 0
	}
	due := time.Unix(l.Iat, 0).Add(renewInterval).Sub(eff)
	return max(0, min(due, renewInterval))
}

func (m *Manager) binding(ctx context.Context) (Binding, error) {
	if m.lid == "" {
		return Binding{}, errors.New("license: unparseable key")
	}
	uid, err := m.liveUID(ctx)
	if err != nil {
		return Binding{}, fmt.Errorf("read kube-system uid: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return Binding{
		LID: m.lid, ClusterUUID: m.clusterUUID(uid),
		ClusterProof: ClusterProof(uid, m.lid), InstancePub: m.pub,
	}, nil
}

// cycle performs one activation/renewal attempt and returns the delay before
// the next one.
func (m *Manager) cycle(ctx context.Context) time.Duration {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.Lock()
	released := m.released
	m.mu.Unlock()
	if released {
		return m.interval()
	}
	if _, err := m.v.VerifyKey(m.cfg.Key, m.effective()); err != nil {
		// An expired key is still renewed while a verified lease extends
		// the license past it; the server delivers extensions that way.
		if !errors.Is(err, ErrExpired) || !m.leaseExtends(ctx) {
			m.refresh(ctx) // invalid or expired key: nothing to ask the server
			return m.interval()
		}
	}
	if err := m.ensureInstance(ctx); err != nil {
		m.refresh(ctx)
		return m.backoff()
	}
	b, err := m.binding(ctx)
	if err != nil {
		klog.ErrorS(err, "license binding unavailable", "component", "license")
		m.refresh(ctx)
		return m.backoff()
	}
	err = m.sync(ctx, b)
	m.refresh(ctx)
	return m.nextDelay(err)
}

// leaseExtends reports whether the cached lease verifies for this cluster
// and carries a license end that is still in the future.
func (m *Manager) leaseExtends(ctx context.Context) bool {
	b, err := m.binding(ctx)
	if err != nil {
		return false
	}
	m.mu.Lock()
	lease := m.lease
	m.mu.Unlock()
	eff := m.effective()
	l, err := m.v.VerifyLease(lease, b, eff)
	return err == nil && l.LicenseEnd().After(eff)
}

func (m *Manager) nextDelay(err error) time.Duration {
	var conflict ErrConflict
	if errors.As(err, &conflict) {
		return m.conflictDelay()
	}
	if err == nil || verdictFor(err) != VerdictNone {
		m.clearConflict()
	}
	if err == nil {
		m.mu.Lock()
		m.failures = 0
		m.mu.Unlock()
		return m.interval()
	}
	klog.V(2).InfoS("license sync failed", "component", "license",
		"err", err.Error())
	var rl ErrRateLimited
	if errors.As(err, &rl) {
		return max(m.backoff(), rl.RetryAfter)
	}
	var se *StatusError
	if verdictFor(err) != VerdictNone ||
		(errors.As(err, &se) && se.Status >= 400 && se.Status < 500) {
		m.mu.Lock()
		m.failures = 0
		m.mu.Unlock()
		return m.interval()
	}
	return m.backoff()
}

func verdictFor(err error) Verdict {
	var c ErrConflict
	switch {
	case errors.As(err, &c), errors.Is(err, ErrProofMismatch):
		return VerdictConflict
	case errors.Is(err, ErrRevoked):
		return VerdictRevoked
	case errors.Is(err, ErrSuspended):
		return VerdictSuspended
	case errors.Is(err, ErrKeyReplaced):
		return VerdictKeyReplaced
	case errors.Is(err, ErrInstanceSuperseded):
		return VerdictSuperseded
	case errors.Is(err, ErrServerExpired):
		return VerdictExpired
	case errors.Is(err, ErrTrialUsed):
		return VerdictTrialUsed
	}
	return VerdictNone
}

func (m *Manager) nodeCount(ctx context.Context) int {
	if m.cfg.NodeCount == nil {
		return 0
	}
	return m.cfg.NodeCount(ctx)
}
