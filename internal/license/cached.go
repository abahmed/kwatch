package license

import "time"

// CachedLeaseGraceUntil reads when the grace of a cached lease ends. It is
// for reading kwatch's own state file from outside the running process,
// where the cluster binding needed to verify the lease is not available,
// so it checks the signature only: a lease not signed by a trusted key
// (an edited state file) reads as no lease. A nil v uses
// DefaultVerifier.
func CachedLeaseGraceUntil(v *Verifier, lease string) (time.Time, bool) {
	if v == nil {
		v = DefaultVerifier()
	}
	l, ok := v.signedLease(lease)
	if !ok {
		return time.Time{}, false
	}
	return l.GraceUntil(), true
}

// NewerLease returns whichever of two cached leases has a verified
// signature and the later iat. A lease that does not verify never wins,
// so a tampered copy cannot displace a good one. A nil v uses
// DefaultVerifier.
func NewerLease(v *Verifier, a, b string) string {
	if v == nil {
		v = DefaultVerifier()
	}
	la, okA := v.signedLease(a)
	lb, okB := v.signedLease(b)
	if okB && (!okA || lb.Iat > la.Iat) || !okA && a == "" {
		return b
	}
	return a
}

// signedLease decodes s and checks its signature, not its binding or
// time window.
func (v *Verifier) signedLease(s string) (Lease, bool) {
	payload, sig, err := split(leasePrefix, s)
	if err != nil {
		return Lease{}, false
	}
	l, err := decodeLease(payload)
	if err != nil || v.check(l.KID, payload, sig) != nil {
		return Lease{}, false
	}
	return l, true
}
