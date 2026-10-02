package storage

import (
	"context"
	"time"
)

// CompactInterval is how often the production compactor runs a pass.
const CompactInterval = 15 * time.Minute

// Compactor enforces a Policy in the background: retention first, then
// the evidence cap, then the total size cap, always oldest first and in
// small transactions. It runs in its own goroutine, never on the
// decision loop, and its writes are fenced like any other write.
type Compactor struct {
	store  *Store
	policy Policy
	passes chan PassResult
}

// PassResult is what one pass removed and the logical size it left.
type PassResult struct {
	// Expired counts entries removed by retention.
	Expired int
	// Evicted counts entries removed to meet a size cap.
	Evicted int
	// Bytes is the total logical size after the pass.
	Bytes int64
	// PinnedBytes is the part of Bytes the size cap may not delete:
	// incidents, baselines, fingerprints, state and threads.
	PinnedBytes int64
	// OverCap is true when the pass left the store over SizeCap because
	// pinned data alone leaves too little room for history. The pass
	// then keeps a minimum of history instead of deleting all of it.
	OverCap bool
	// FileBytes is the size of the state file after the pass.
	FileBytes int64
	// RewriteDue is true when the file is so far over SizeCap that the
	// next start rewrites it (see physical.go).
	RewriteDue bool
}

// minHistoryDivisor sets the history the size cap always keeps: a
// quarter of the cap, however much pinned data there is.
const minHistoryDivisor = 4

// NewCompactor returns a compactor for store. Zero policy fields take
// their defaults.
func NewCompactor(store *Store, policy Policy) *Compactor {
	if policy.Batch <= 0 {
		policy.Batch = defaultBatch
	}
	policy.SizeCap = capOrDefault(policy.SizeCap)
	return &Compactor{
		store: store, policy: policy, passes: make(chan PassResult, 1),
	}
}

// Passes receives the result of each finished pass; it is the
// compactor's progress signal. Results are dropped when nobody listens.
func (c *Compactor) Passes() <-chan PassResult { return c.passes }

// Run passes once at start and then on every tick until ctx ends. It
// returns nil on cancellation, within one batch transaction, and the
// error of a failed pass otherwise (for example ErrFenced).
func (c *Compactor) Run(ctx context.Context, tick <-chan time.Time) error {
	for {
		result, err := c.Pass(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case c.passes <- result:
		default:
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
		}
	}
}

// Pass runs retention and both caps once.
func (c *Compactor) Pass(ctx context.Context) (PassResult, error) {
	var result PassResult
	expired, err := c.expire(ctx)
	result.Expired = expired
	if err != nil {
		return result, err
	}
	fileBytes, _ := fileSize(c.store.path)
	evicted, err := c.enforceCaps(ctx, fileBytes)
	result.Evicted = evicted
	if err != nil {
		return result, err
	}
	report, err := c.store.measure(ctx)
	if err != nil {
		return result, err
	}
	result.Bytes, result.PinnedBytes = report.total(), report.pinned()
	result.OverCap = result.Bytes > c.policy.SizeCap
	c.store.counters.overCap.Store(result.OverCap)
	result.FileBytes, _ = fileSize(c.store.path)
	result.RewriteDue = oversized(c.store.path, c.policy.SizeCap)
	c.store.noteRewriteDue(result.RewriteDue, result.FileBytes)
	return result, nil
}

func (c *Compactor) expire(ctx context.Context) (int, error) {
	now := c.store.now()
	removed := 0
	for _, sp := range specs {
		expired := keyedExpired(now)
		if sp.shape == logged {
			keep := c.policy.Retention[sp.name]
			if keep <= 0 {
				continue
			}
			expired = logExpired(now.Add(-keep))
		}
		n, err := c.store.expireBucket(
			ctx, sp.name, c.policy.Batch, expired)
		removed += n
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// enforceCaps applies the evidence cap, then the total cap. Only
// evictable bytes can be deleted, so the total cap gives history the
// room pinned data leaves, but never less than a quarter of the cap:
// pinned data over the cap must not wipe all history on every pass. A
// file of fileBytes over the cap lowers the total cap by the excess.
func (c *Compactor) enforceCaps(
	ctx context.Context, fileBytes int64,
) (int, error) {
	report, err := c.store.measure(ctx)
	if err != nil {
		return 0, err
	}
	evicted := 0
	over := report.buckets[Evidence] - c.policy.EvidenceCap
	if c.policy.EvidenceCap > 0 && over > 0 {
		n, err := c.store.evictOldest(
			ctx, []Bucket{Evidence}, over, c.policy.Batch)
		evicted += n
		if err != nil {
			return evicted, err
		}
		if report, err = c.store.measure(ctx); err != nil {
			return evicted, err
		}
	}
	over = report.evictable - c.historyBudget(report.pinned(), fileBytes)
	if over <= 0 {
		return evicted, nil
	}
	n, err := c.store.evictOldest(ctx, evictable, over, c.policy.Batch)
	return evicted + n, err
}

// historyBudget is how many evictable bytes the total cap keeps.
func (c *Compactor) historyBudget(pinned, fileBytes int64) int64 {
	floor := c.policy.SizeCap / minHistoryDivisor
	target := physicalTarget(c.policy.SizeCap, fileBytes)
	return max(target-pinned, floor)
}
