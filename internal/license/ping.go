package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// maxPingBody is the server's limit for a /license/ping body.
const maxPingBody = 1024

// pingStateStopping marks the last check-in of a graceful shutdown.
const pingStateStopping = "stopping"

// Ping is one cluster check-in. It carries no cluster-identifying data:
// no cluster name, namespace or resource names. Checkins false tells the
// server that the operator turned check-ins off; Stopping marks a graceful
// shutdown.
type Ping struct {
	Checkins bool
	Stopping bool
}

type pingBody struct {
	LID          string `json:"lid"`
	ClusterUUID  string `json:"cluster_uuid"`
	ClusterProof string `json:"cluster_proof"`
	TS           int64  `json:"ts"`
	Version      string `json:"kwatch_version"`
	NodeCount    int    `json:"node_count"`
	Checkins     bool   `json:"checkins"`
	State        string `json:"state,omitempty"`
}

// ErrPingTooLarge means a check-in body would exceed the server limit.
var ErrPingTooLarge = errors.New("license: check-in body too large")

// Ping sends a check-in to POST /license/ping, signed like a renewal. The
// server answers 204; errors are classified as for Renew.
func (c *Client) Ping(
	ctx context.Context, priv ed25519.PrivateKey, req SignedRequest,
	ping Ping,
) error {
	if len(priv) != ed25519.PrivateKeySize {
		return errors.New("license: invalid instance key")
	}
	state := ""
	if ping.Stopping {
		state = pingStateStopping
	}
	body, err := json.Marshal(pingBody{
		LID: req.LID, ClusterUUID: req.ClusterUUID,
		ClusterProof: req.ClusterProof, TS: req.TS.Unix(),
		Version: req.Version, NodeCount: max(req.NodeCount, 0),
		Checkins: ping.Checkins, State: state,
	})
	if err != nil {
		return err
	}
	if len(body) > maxPingBody {
		return ErrPingTooLarge
	}
	sig := base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, body))
	_, err = c.call(ctx, "/license/ping", body, sig, false)
	return err
}

// Ping sends a check-in for this cluster's binding. It needs a key and the
// instance key; it does not take the network lock, so a shutdown check-in
// never waits behind a slow renewal.
func (m *Manager) Ping(ctx context.Context, ping Ping) error {
	if m.cfg.Key == "" {
		return errors.New("license: no key")
	}
	if err := m.ensureInstance(ctx); err != nil {
		return err
	}
	b, err := m.binding(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	priv := m.priv
	m.mu.Unlock()
	return m.cfg.Client.Ping(ctx, priv, m.signedRequest(ctx, b, 0), ping)
}

// StopsCheckins reports a ping answer that means the server will not take
// check-ins for this license until it changes: 401, 403 and 409 verdicts.
func StopsCheckins(err error) bool {
	var conflict ErrConflict
	return errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrRevoked) ||
		errors.Is(err, ErrSuspended) ||
		errors.Is(err, ErrServerExpired) ||
		errors.Is(err, ErrProofMismatch) ||
		errors.Is(err, ErrInstanceSuperseded) ||
		errors.As(err, &conflict) || isForbidden(err)
}

func isForbidden(err error) bool {
	var status *StatusError
	return errors.As(err, &status) &&
		(status.Status == 403 || status.Status == 409)
}
