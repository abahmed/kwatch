package storage

import (
	"context"
	"time"

	bolt "go.etcd.io/bbolt"
)

// defaultBatch bounds one compaction write transaction so the write lock
// is never held long enough to delay the decision loop's writes.
const defaultBatch = 500

// scanLimit bounds how many keys one read transaction of the retention
// scan visits, so a huge bucket never pins one long read transaction.
const scanLimit = 50 * defaultBatch

// ctxCheckEvery is how many keys a scan visits between context checks.
const ctxCheckEvery = 1024

// evictable are the buckets whose data may be dropped to meet the total
// size cap: the history logs. Keyed buckets are never evicted. Resolved
// incidents are already bounded by their expiry and the incident
// manager's caps, and the incidents bucket is a mirror of the manager:
// evicting from it would only make the next save write the records
// back, over and over.
//
// The outbox is not evictable on purpose. The delivery manager already
// bounds it (outboxMaxEntries records, and records older than
// outboxMaxAge are dropped at restore), so it cannot grow without limit.
// Evicting it here would silently lose queued notifications, which is
// worse than the few megabytes it holds.
var evictable = []Bucket{Evidence, Audit, Timeline, Changes}

// isEvictable reports whether the size cap may delete from b.
func isEvictable(b Bucket) bool {
	for _, e := range evictable {
		if e == b {
			return true
		}
	}
	return false
}

// expiredFunc reports whether one stored entry should be deleted.
type expiredFunc func(key, data []byte) bool

// expireBucket deletes the expired entries of b. It finds them in read
// transactions and opens a write transaction only for a batch that has
// something to delete, so a pass over unexpired data writes nothing.
func (s *Store) expireBucket(
	ctx context.Context, b Bucket, batch int, expired expiredFunc,
) (int, error) {
	var from []byte
	total := 0
	for {
		doomed, next, err := s.scanExpired(ctx, b, from, batch, expired)
		if err != nil {
			return total, err
		}
		removed, err := s.deleteExpired(b, doomed, expired)
		total += removed
		if err != nil || next == nil {
			return total, err
		}
		from = next
	}
}

// scanExpired reads keys of b from the key from (the first key when
// from is nil) until it has found batch expired keys or visited
// scanLimit keys. It returns the expired keys and the first key it did
// not inspect, which is nil when it reached the end of the bucket.
func (s *Store) scanExpired(
	ctx context.Context, b Bucket, from []byte, batch int,
	expired expiredFunc,
) (doomed [][]byte, next []byte, err error) {
	s.counters.scans.Add(1)
	err = s.view(func(tx *bolt.Tx) error {
		cursor := tx.Bucket([]byte(b)).Cursor()
		doomed, next, err = collectExpired(
			ctx, cursor, from, batch, expired)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return doomed, next, nil
}

// collectExpired is the cursor walk of scanExpired.
func collectExpired(
	ctx context.Context, cursor *bolt.Cursor, from []byte, batch int,
	expired expiredFunc,
) ([][]byte, []byte, error) {
	var doomed [][]byte
	k, v := cursor.First()
	if from != nil {
		k, v = cursor.Seek(from)
	}
	for visited := 0; k != nil; visited++ {
		if visited%ctxCheckEvery == 0 && ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if len(doomed) >= batch || visited >= scanLimit {
			return doomed, append([]byte(nil), k...), nil
		}
		if expired(k, v) {
			doomed = append(doomed, append([]byte(nil), k...))
		}
		k, v = cursor.Next()
	}
	return doomed, nil, nil
}

// deleteExpired deletes doomed keys of b that are still expired. A key
// rewritten since the scan, for example an incident that reopened, is
// checked again inside the write transaction and kept.
func (s *Store) deleteExpired(
	b Bucket, doomed [][]byte, expired expiredFunc,
) (int, error) {
	if len(doomed) == 0 {
		return 0, nil
	}
	removed := 0
	err := s.update(func(tx *bolt.Tx) error {
		s.touch(b)
		bucket := tx.Bucket([]byte(b))
		for _, key := range doomed {
			data := bucket.Get(key)
			if data == nil || !expired(key, data) {
				continue
			}
			if err := bucket.Delete(key); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.counters.expired.Add(uint64(removed))
	return removed, nil
}

// logExpired deletes log entries older than cutoff.
func logExpired(cutoff time.Time) expiredFunc {
	return func(key, _ []byte) bool {
		at, ok := logTime(key)
		return ok && at.Before(cutoff)
	}
}

// keyedExpired deletes keyed values whose expiry has passed. A value
// whose header is unreadable is kept: it is skipped on read instead.
func keyedExpired(now time.Time) expiredFunc {
	return func(_, data []byte) bool {
		h, ok := readHeader(data)
		return ok && h.expired(now)
	}
}

func shapeOf(b Bucket) shape {
	for _, sp := range specs {
		if sp.name == b {
			return sp.shape
		}
	}
	return keyed
}

// sizeReport is the logical size of the store, split by what the size
// cap may delete.
type sizeReport struct {
	// buckets is the key and value bytes stored per bucket.
	buckets map[Bucket]int64
	// evictable is the part of the total the size cap may delete.
	evictable int64
}

func (r sizeReport) total() int64 { return total(r.buckets) }

// pinned is the part of the total the size cap must keep: open
// incidents, baselines, fingerprints, state and threads.
func (r sizeReport) pinned() int64 { return r.total() - r.evictable }

// measure reads the logical size of every bucket in one scan.
func (s *Store) measure(ctx context.Context) (sizeReport, error) {
	report := sizeReport{buckets: make(map[Bucket]int64, len(specs))}
	s.counters.scans.Add(1)
	err := s.view(func(tx *bolt.Tx) error {
		visited := 0
		for _, sp := range specs {
			bucket := tx.Bucket([]byte(sp.name))
			if bucket == nil {
				continue // a read-only file from before this bucket
			}
			err := report.add(ctx, sp.name, bucket.Cursor(), &visited)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return report, err
}

// add counts every entry under cursor into the report for bucket b.
// visited counts keys across buckets, for the context checks.
func (r *sizeReport) add(
	ctx context.Context, b Bucket, cursor *bolt.Cursor, visited *int,
) error {
	for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
		if *visited++; *visited%ctxCheckEvery == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		size := int64(len(k) + len(v))
		r.buckets[b] += size
		if _, ok := evictableAt(b, k, v); ok {
			r.evictable += size
		}
	}
	return nil
}

// LogicalSize returns the key and value bytes stored per bucket. It is
// what the size caps measure: bbolt never shrinks its file, so the file
// size says little about how much data is live.
func (s *Store) LogicalSize() (map[Bucket]int64, error) {
	report, err := s.measure(context.Background())
	return report.buckets, err
}

// FileSize returns the state file's data size in bytes.
func (s *Store) FileSize() (int64, error) {
	var size int64
	err := s.view(func(tx *bolt.Tx) error {
		size = tx.Size()
		return nil
	})
	return size, err
}

func total(sizes map[Bucket]int64) int64 {
	var sum int64
	for _, size := range sizes {
		sum += size
	}
	return sum
}
