package license

import (
	"context"
	"errors"
	"time"
)

// serverNow is the local clock corrected by the last known server offset.
// It only timestamps signed requests; license decisions never use it.
func (m *Manager) serverNow() time.Time {
	m.mu.Lock()
	off := m.offset
	m.mu.Unlock()
	return m.cfg.Now().Add(off)
}

// noteServerTime records the offset between srv (a response's unsigned
// server time) and the local clock. Zero srv leaves the offset alone.
func (m *Manager) noteServerTime(srv time.Time) {
	if srv.IsZero() {
		return
	}
	m.mu.Lock()
	m.offset = srv.Sub(m.cfg.Now()).Round(time.Second)
	m.mu.Unlock()
}

// signedCall makes one instance-signed request timestamped with the
// corrected clock. A stale_timestamp answer that carries the server time
// resyncs the offset and is retried once; a second rejection is returned
// as is and handled like any transient failure, so it never changes the
// licensing state.
func (m *Manager) signedCall(
	ctx context.Context, b Binding, leaseIat int64,
	call func(SignedRequest) (Result, error),
) (Result, error) {
	res, err := call(m.signedRequest(ctx, b, leaseIat))
	m.noteServerTime(res.SrvTime)
	if errors.Is(err, ErrStaleTimestamp) && !res.SrvTime.IsZero() &&
		ctx.Err() == nil {
		res, err = call(m.signedRequest(ctx, b, leaseIat))
		m.noteServerTime(res.SrvTime)
	}
	return res, err
}

// timedActivate sends an activation or move stamped with the corrected
// clock, with the same single stale_timestamp retry as signedCall.
func (m *Manager) timedActivate(
	ctx context.Context, b Binding,
	call func(context.Context, ActivateRequest) (Result, error),
) (Result, error) {
	n := m.nodeCount(ctx)
	req := func() ActivateRequest {
		return ActivateRequest{
			Key: m.cfg.Key, ClusterUUID: b.ClusterUUID,
			ClusterProof: b.ClusterProof, InstancePub: b.InstancePub,
			Version: m.cfg.Version, NodeCount: n, TS: m.serverNow(),
		}
	}
	res, err := call(ctx, req())
	m.noteServerTime(res.SrvTime)
	if errors.Is(err, ErrStaleTimestamp) && !res.SrvTime.IsZero() &&
		ctx.Err() == nil {
		res, err = call(ctx, req())
		m.noteServerTime(res.SrvTime)
	}
	return res, err
}
