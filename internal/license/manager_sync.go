package license

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/klog/v2"
)

func (m *Manager) signedRequest(
	ctx context.Context, b Binding, leaseIat int64,
) SignedRequest {
	return SignedRequest{
		LID: b.LID, ClusterUUID: b.ClusterUUID, ClusterProof: b.ClusterProof,
		LeaseIat: leaseIat, TS: m.cfg.Now(),
		Version: m.cfg.Version, NodeCount: m.nodeCount(ctx),
	}
}

func (m *Manager) activate(ctx context.Context, b Binding) error {
	res, err := m.cfg.Client.Activate(ctx, ActivateRequest{
		Key: m.cfg.Key, ClusterUUID: b.ClusterUUID, ClusterProof: b.ClusterProof,
		InstancePub: b.InstancePub,
		Version:     m.cfg.Version, NodeCount: m.nodeCount(ctx),
	})
	return m.handle(res, err, b)
}

// sync renews a valid cached lease, or activates when there is none (or a
// server verdict says the binding is in doubt).
func (m *Manager) sync(ctx context.Context, b Binding) error {
	m.mu.Lock()
	lease, verdict, priv := m.lease, m.verdict, m.priv
	m.mu.Unlock()

	cur, err := m.v.VerifyLease(lease, b, m.effective())
	renewable := err == nil &&
		(verdict == VerdictNone || verdict == VerdictSuperseded)
	if !renewable {
		return m.activate(ctx, b)
	}
	res, err := m.cfg.Client.Renew(ctx, priv, m.signedRequest(ctx, b, cur.Iat))
	if errors.Is(err, ErrUnauthorized) {
		// The binding is gone (for example after a transfer): activate once.
		return m.activate(ctx, b)
	}
	return m.handle(res, err, b)
}

// handle applies a server response. Only the srv_time inside a lease that
// verified moves the clock guard; the response body's own srv_time is
// unsigned and error responses are never trusted for time.
func (m *Manager) handle(res Result, callErr error, b Binding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if callErr != nil {
		if v := verdictFor(callErr); v != VerdictNone {
			m.verdict = v
		}
		m.persistLocked()
		return callErr
	}
	lease, err := m.v.verifyLeaseSigned(res.Lease, b)
	if err == nil {
		if lease.SrvTime > 0 {
			m.clock.Observe(time.Unix(lease.SrvTime, 0).UTC())
		}
		_, err = checkLeaseWindow(lease, m.clock.Effective(m.cfg.Now()))
	}
	if err != nil {
		m.persistLocked()
		return fmt.Errorf("license: server returned an unusable lease: %w", err)
	}
	m.lease, m.verdict = res.Lease, VerdictNone
	m.persistLocked()
	return nil
}

func (m *Manager) persistLocked() {
	if m.cfg.LeaseStore == nil {
		return
	}
	if err := m.cfg.LeaseStore.SaveLease(m.lease, m.clock); err != nil {
		klog.ErrorS(err, "persist license lease", "component", "license")
		return
	}
	m.savedMax = m.clock.MaxSeen
}

// Deactivate releases this cluster's binding on the server and drops the
// local lease; the renew loop then stops contacting the server. It does
// nothing without a key.
func (m *Manager) Deactivate(ctx context.Context) error {
	if m.cfg.Key == "" {
		return nil
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if err := m.ensureInstance(ctx); err != nil {
		return err
	}
	b, err := m.binding(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	lease, priv := m.lease, m.priv
	m.mu.Unlock()
	var iat int64
	if l, err := m.v.VerifyLease(lease, b, m.effective()); err == nil {
		iat = l.Iat
	}
	_, err = m.cfg.Client.Deactivate(ctx, priv, m.signedRequest(ctx, b, iat))
	if err != nil && !errors.Is(err, ErrUnauthorized) {
		m.mu.Lock()
		m.persistLocked()
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	m.lease, m.verdict, m.released = "", VerdictNone, true
	m.persistLocked()
	m.mu.Unlock()
	m.refresh(ctx)
	return nil
}

// CheckDeactivated picks up a release made by another process (see
// Config.DeactivatedAt): the cached lease is dropped and the status falls
// back to pending, which entitles Community. It is cheap to call on every
// status tick.
func (m *Manager) CheckDeactivated(ctx context.Context) {
	if m.applyDeactivation(ctx) {
		m.refresh(ctx)
	}
}

func (m *Manager) applyDeactivation(ctx context.Context) bool {
	if m.cfg.Key == "" || m.cfg.DeactivatedAt == nil {
		return false
	}
	at, err := m.cfg.DeactivatedAt(ctx)
	if err != nil {
		klog.V(2).InfoS("read license deactivation marker failed",
			"component", "license", "err", err.Error())
		return false
	}
	if at.IsZero() {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	if iat, ok := leaseIssuedAt(m.lease); ok && iat <= at.Unix() {
		m.lease, m.verdict = "", VerdictNone
		m.persistLocked()
		changed = true
	}
	if !at.Before(m.cfg.ProcessStart) && !m.released {
		m.released, changed = true, true
	}
	return changed
}

// leaseIssuedAt reads iat from our own cached lease without verifying it:
// the value only decides whether to drop that lease.
func leaseIssuedAt(s string) (int64, bool) {
	payload, _, err := split(leasePrefix, s)
	if err != nil {
		return 0, false
	}
	l, err := decodeLease(payload)
	return l.Iat, err == nil
}
