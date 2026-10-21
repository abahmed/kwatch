package license

import "time"

// CachedLeaseGraceUntil reads, without verifying it, when the grace of a
// cached lease ends. It is for reading kwatch's own state file from
// outside the running process, where the cluster binding needed to verify
// the lease is not available; the running leader has already verified the
// lease, and a caller that gates behavior still goes through Evaluate.
func CachedLeaseGraceUntil(lease string) (time.Time, bool) {
	payload, _, err := split(leasePrefix, lease)
	if err != nil {
		return time.Time{}, false
	}
	l, err := decodeLease(payload)
	if err != nil {
		return time.Time{}, false
	}
	return l.GraceUntil(), true
}
