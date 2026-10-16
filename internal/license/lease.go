package license

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Grace is how long a lease keeps working after it expires. The server
// issues 3 day leases, so a cluster runs at most 7 days without contact.
const Grace = 4 * 24 * time.Hour

// Errors returned by lease verification.
var (
	ErrBindingMismatch  = errors.New("license: lease does not match this cluster")
	ErrLeaseNotYetValid = errors.New("license: lease not yet valid")
	ErrLeaseExpired     = errors.New("license: lease expired")
)

// Lease is the signed payload of an activation lease.
type Lease struct {
	KID          string `json:"kid"`
	LID          string `json:"lid"`
	ClusterUUID  string `json:"cluster_uuid"`
	ClusterProof string `json:"cluster_proof"`
	InstancePub  string `json:"instance_pub"`
	Plan         string `json:"plan"`
	LicExp       int64  `json:"lic_exp"`
	Iat          int64  `json:"iat"`
	Nbf          int64  `json:"nbf"`
	Exp          int64  `json:"exp"`
	SrvTime      int64  `json:"srv_time"`

	// InGrace is set by VerifyLease, never serialized.
	InGrace bool `json:"-"`
}

// Binding is what a lease must be bound to on this installation.
type Binding struct {
	LID          string
	ClusterUUID  string
	ClusterProof string
	InstancePub  string
}

// ExpiresAt returns the lease expiry time.
func (l Lease) ExpiresAt() time.Time { return time.Unix(l.Exp, 0).UTC() }

// GraceUntil returns the end of the post-expiry grace window.
func (l Lease) GraceUntil() time.Time { return l.ExpiresAt().Add(Grace) }

// LicenseEnd returns the end of the license as the server last stated it
// (lic_exp); zero when the lease carries none.
func (l Lease) LicenseEnd() time.Time {
	if l.LicExp <= 0 {
		return time.Time{}
	}
	return time.Unix(l.LicExp, 0).UTC()
}

func signLease(l Lease, priv ed25519.PrivateKey) string {
	payload, _ := json.Marshal(l)
	return sign(leasePrefix, payload, priv)
}

func decodeLease(payload []byte) (Lease, error) {
	var l Lease
	if err := json.Unmarshal(payload, &l); err != nil {
		return Lease{}, malformed("bad payload json")
	}
	if l.KID == "" || l.LID == "" || l.Exp <= 0 || l.Nbf <= 0 {
		return Lease{}, malformed("missing required fields")
	}
	return l, nil
}

func (l Lease) matches(want Binding) bool {
	return l.LID == want.LID &&
		l.ClusterUUID == want.ClusterUUID &&
		l.ClusterProof == want.ClusterProof &&
		l.InstancePub == want.InstancePub
}

// VerifyLease verifies the lease signature, its binding to want and its
// validity window at now. Between exp and exp+Grace the lease is returned
// with InGrace set; after that ErrLeaseExpired is returned.
func (v *Verifier) VerifyLease(
	s string, want Binding, now time.Time,
) (Lease, error) {
	l, err := v.verifyLeaseSigned(s, want)
	if err != nil {
		return Lease{}, err
	}
	return checkLeaseWindow(l, now)
}

// verifyLeaseSigned checks the signature and the binding, not the time.
func (v *Verifier) verifyLeaseSigned(s string, want Binding) (Lease, error) {
	payload, sig, err := split(leasePrefix, s)
	if err != nil {
		return Lease{}, err
	}
	l, err := decodeLease(payload)
	if err != nil {
		return Lease{}, err
	}
	if err := v.check(l.KID, payload, sig); err != nil {
		return Lease{}, err
	}
	if !l.matches(want) {
		return Lease{}, ErrBindingMismatch
	}
	return l, nil
}

func checkLeaseWindow(l Lease, now time.Time) (Lease, error) {
	if now.Before(time.Unix(l.Nbf, 0)) {
		return Lease{}, ErrLeaseNotYetValid
	}
	switch {
	case !now.Before(l.GraceUntil()):
		return l, fmt.Errorf(
			"%w: expired %s", ErrLeaseExpired,
			l.ExpiresAt().Format(time.RFC3339),
		)
	case !now.Before(l.ExpiresAt()):
		l.InGrace = true
	}
	return l, nil
}
