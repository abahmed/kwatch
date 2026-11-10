package storage

import "time"

// A rewrite that fails (no space, a copy that will not open or rename)
// would fail again on the next pass, and each try copies the whole file
// while writes wait on the gate. After a failure the store waits
// reclaimRetryMin, doubling per failure up to reclaimRetryMax, and a
// rewrite that succeeds starts over.
const (
	reclaimRetryMin = 10 * time.Minute
	reclaimRetryMax = 6 * time.Hour
)

// reclaimBackoff is the wait after failed rewrites. It is held under
// the file gate.
type reclaimBackoff struct {
	failures int
	retryAt  time.Time
}

// waiting reports whether a failure still holds the next rewrite back.
func (b *reclaimBackoff) waiting(now time.Time) bool {
	return b.failures > 0 && now.Before(b.retryAt)
}

// fail records a failed rewrite at now.
func (b *reclaimBackoff) fail(now time.Time) {
	wait := reclaimRetryMin
	for i := 0; i < b.failures && wait < reclaimRetryMax; i++ {
		wait *= 2
	}
	b.failures++
	b.retryAt = now.Add(min(wait, reclaimRetryMax))
}

// reset forgets the failures after a rewrite succeeded.
func (b *reclaimBackoff) reset() { *b = reclaimBackoff{} }
