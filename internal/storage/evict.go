package storage

import (
	"container/heap"
	"context"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

// victimsPerBatch bounds how many victims one scan keeps in memory, as a
// multiple of the policy batch. A scan finds enough victims for many
// delete transactions, so a large overshoot costs few full scans.
const victimsPerBatch = 20

// victim is one entry the size cap may delete.
type victim struct {
	bucket Bucket
	key    []byte
	at     time.Time
	size   int64
}

// victimHeap keeps the oldest entries seen so far. The youngest kept
// entry is on top, so it is the first to make way for an older one.
type victimHeap struct {
	items []victim
	bytes int64
}

func (h victimHeap) Len() int { return len(h.items) }

func (h victimHeap) Less(i, j int) bool {
	return h.items[i].at.After(h.items[j].at)
}

func (h victimHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *victimHeap) Push(x any) { h.items = append(h.items, x.(victim)) }

func (h *victimHeap) Pop() any {
	last := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	return last
}

// offer adds v and then drops the youngest entries the heap does not
// need: those beyond limit, and those not needed to free over bytes.
func (h *victimHeap) offer(v victim, over int64, limit int) {
	heap.Push(h, v)
	h.bytes += v.size
	for h.Len() > 1 &&
		(h.Len() > limit || h.bytes-h.items[0].size >= over) {
		h.bytes -= heap.Pop(h).(victim).size
	}
}

// evictOldest deletes the oldest evictable entries of buckets until at
// least over bytes are freed or nothing evictable is left. One scan
// finds the victims for many batches; each batch is one transaction.
func (s *Store) evictOldest(
	ctx context.Context, buckets []Bucket, over int64, batch int,
) (int, error) {
	total := 0
	for over > 0 {
		victims, err := s.collectVictims(
			ctx, buckets, over, victimsPerBatch*batch)
		if err != nil || len(victims) == 0 {
			return total, err
		}
		freed, removed, err := s.deleteVictims(ctx, victims, batch)
		total += removed
		over -= freed
		if err != nil || removed == 0 {
			return total, err
		}
	}
	return total, nil
}

// collectVictims scans buckets once and returns, oldest first, the
// oldest evictable entries that together free over bytes, at most limit.
func (s *Store) collectVictims(
	ctx context.Context, buckets []Bucket, over int64, limit int,
) ([]victim, error) {
	h := &victimHeap{}
	s.counters.scans.Add(1)
	err := s.view(func(tx *bolt.Tx) error {
		visited := 0
		for _, b := range buckets {
			cursor := tx.Bucket([]byte(b)).Cursor()
			for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
				if visited++; visited%ctxCheckEvery == 0 &&
					ctx.Err() != nil {
					return ctx.Err()
				}
				if c, ok := candidate(b, k, v); ok {
					h.offer(c, over, limit)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(h.items, func(i, j int) bool {
		return h.items[i].at.Before(h.items[j].at)
	})
	return h.items, nil
}

// deleteVictims deletes victims in transactions of at most batch keys.
// An entry rewritten since the scan is no longer the victim that was
// chosen and is kept.
func (s *Store) deleteVictims(
	ctx context.Context, victims []victim, batch int,
) (freed int64, removed int, err error) {
	for start := 0; start < len(victims); start += batch {
		if err := ctx.Err(); err != nil {
			return freed, removed, err
		}
		end := min(start+batch, len(victims))
		f, n, err := s.deleteVictimBatch(victims[start:end])
		freed, removed = freed+f, removed+n
		if err != nil {
			return freed, removed, err
		}
	}
	return freed, removed, nil
}

func (s *Store) deleteVictimBatch(victims []victim) (int64, int, error) {
	var freed int64
	removed := 0
	err := s.update(func(tx *bolt.Tx) error {
		freed, removed = 0, 0
		for _, v := range victims {
			bucket := tx.Bucket([]byte(v.bucket))
			now, ok := candidate(v.bucket, v.key, bucket.Get(v.key))
			if !ok || !now.at.Equal(v.at) {
				continue
			}
			s.touch(v.bucket)
			if err := bucket.Delete(v.key); err != nil {
				return err
			}
			freed += v.size
			removed++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	s.counters.evicted.Add(uint64(removed))
	return freed, removed, nil
}

// candidate describes an evictable entry: a log entry of an evictable
// bucket. A missing value is not a candidate.
func candidate(b Bucket, key, data []byte) (victim, bool) {
	at, ok := evictableAt(b, key, data)
	if !ok {
		return victim{}, false
	}
	size := int64(len(key) + len(data))
	return victim{b, append([]byte(nil), key...), at, size}, true
}

// evictableAt reports whether an entry is evictable and its age, the
// entry time of the log entry.
func evictableAt(b Bucket, key, data []byte) (time.Time, bool) {
	if data == nil || !isEvictable(b) || shapeOf(b) != logged {
		return time.Time{}, false
	}
	return logTime(key)
}
