package license

import (
	"context"
	"errors"
	"time"

	"k8s.io/klog/v2"
)

// Results of a requested move.
const (
	MoveMoved         = "moved"
	MoveAlreadyHere   = "already_here"
	MoveTransferLimit = "transfer_limit"
	MoveInvalidKey    = "invalid_key"
	MoveFailed        = "failed"
	MoveStale         = "stale"
)

// moveRequestTTL is how long a request waits for the leader. An older one
// is dropped unanswered by the move itself: a restarted pod must not act on
// a request the owner has long forgotten.
const moveRequestTTL = 10 * time.Minute

// MoveOutcome is how a requested move ended.
type MoveOutcome struct {
	Result string
	// RetryAfter is, for MoveTransferLimit, how long until a slot frees;
	// zero when the server did not say.
	RetryAfter time.Duration
	// Detail carries the error text of MoveFailed.
	Detail string
}

// CheckMove performs a move requested by another process (the `kwatch
// license move` command, see Config.MoveRequestedAt): it takes the license
// over from the cluster it is bound to and adopts the lease. A network
// failure keeps the request for the next call; any other outcome is
// reported once through Config.MoveFinished. It is cheap to call on every
// status tick.
func (m *Manager) CheckMove(ctx context.Context) {
	if m.cfg.Key == "" || m.cfg.MoveRequestedAt == nil ||
		m.cfg.MoveFinished == nil {
		return
	}
	at, err := m.cfg.MoveRequestedAt(ctx)
	if err != nil {
		klog.V(2).InfoS("read license move marker failed",
			"component", "license", "err", err.Error())
		return
	}
	m.mu.Lock()
	handled := !at.After(m.moveHandled)
	m.mu.Unlock()
	if at.IsZero() || handled {
		return
	}
	out, final := MoveOutcome{Result: MoveStale}, true
	if m.cfg.Now().Sub(at) <= moveRequestTTL {
		out, final = m.performMove(ctx)
	}
	if !final {
		return
	}
	m.mu.Lock()
	m.moveHandled = at
	m.mu.Unlock()
	klog.InfoS("license move finished", "component", "license",
		"result", out.Result)
	if err := m.cfg.MoveFinished(ctx, out); err != nil {
		klog.ErrorS(err, "record license move result",
			"component", "license")
	}
}

// performMove asks the server to move the license here. final is false when
// the attempt should be repeated on the next call.
func (m *Manager) performMove(
	ctx context.Context,
) (out MoveOutcome, final bool) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if err := m.ensureInstance(ctx); err != nil {
		return MoveOutcome{}, false
	}
	b, err := m.binding(ctx)
	if err != nil {
		return MoveOutcome{}, false
	}
	here := m.holdsLease(b)
	res, err := m.timedActivate(ctx, b, m.cfg.Client.Move)
	var limit ErrTransferLimit
	var se *StatusError
	switch {
	case errors.As(err, &limit):
		return MoveOutcome{Result: MoveTransferLimit,
			RetryAfter: limit.RetryAfter}, true
	case errors.As(err, &se) && se.Code == "invalid_key":
		return MoveOutcome{Result: MoveInvalidKey}, true
	case err != nil && IsTransient(err):
		return MoveOutcome{}, false
	}
	err = m.handle(res, err, b)
	m.refresh(ctx)
	if err != nil {
		return MoveOutcome{Result: MoveFailed, Detail: err.Error()}, true
	}
	m.mu.Lock()
	m.released, m.movedAt = false, m.cfg.Now()
	m.mu.Unlock()
	m.clearConflict()
	m.refresh(ctx)
	if here {
		return MoveOutcome{Result: MoveAlreadyHere}, true
	}
	return MoveOutcome{Result: MoveMoved}, true
}

// holdsLease reports whether this cluster already has a good lease and no
// adverse server verdict.
func (m *Manager) holdsLease(b Binding) bool {
	m.mu.Lock()
	lease, verdict := m.lease, m.verdict
	m.mu.Unlock()
	if verdict != VerdictNone {
		return false
	}
	_, err := m.v.VerifyLease(lease, b, m.effective())
	return err == nil
}
