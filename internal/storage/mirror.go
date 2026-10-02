package storage

import (
	"crypto/sha256"
	"sync"
	"sync/atomic"

	bolt "go.etcd.io/bbolt"
)

// digest identifies one stored value by its expiry and payload. The
// write time is left out, so saving an unchanged value is a no-op.
type digest [sha256.Size]byte

func digestOf(data []byte) digest {
	return sha256.Sum256(data[8:])
}

// Diff counts what one Mirror.Replace wrote.
type Diff struct {
	// Put counts keys written because they were new or changed.
	Put int
	// Deleted counts keys removed because items no longer named them.
	Deleted int
}

// Mirror keeps a keyed bucket equal to the latest complete snapshot a
// caller hands it, while writing only what changed. It remembers a
// digest of every value it wrote. A write by anyone else to the bucket
// (another view, or the compactor deleting expired values) bumps the
// bucket's write generation, and the next Replace re-reads the bucket
// before comparing. One Mirror must be the bucket's only snapshot
// writer; it is safe for concurrent use.
type Mirror[T any] struct {
	keyed Keyed[T]

	mu      sync.Mutex
	written map[string]digest
	gen     uint64 // bucket write generation written matches
	known   bool   // false until written was read or written once
}

// NewMirror returns a mirror writing through keyed.
func NewMirror[T any](keyed Keyed[T]) *Mirror[T] {
	return &Mirror[T]{keyed: keyed}
}

// Replace makes items the complete content of the bucket in one
// transaction: changed and new keys are written, keys absent from items
// are deleted, and unchanged keys are not touched. Nothing is written
// when one item is invalid or nothing changed.
func (m *Mirror[T]) Replace(items map[string]Item[T]) (Diff, error) {
	encoded, next, err := m.encode(items)
	if err != nil {
		return Diff{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	gen := m.keyed.store.generation(m.keyed.bucket)
	if m.known && gen.Load() == m.gen &&
		unchanged(m.written, next) {
		return Diff{}, nil
	}
	var diff Diff
	var after uint64
	err = m.keyed.store.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(m.keyed.bucket))
		onDisk := m.written
		if !m.known || gen.Load() != m.gen {
			onDisk = digestsOf(bucket)
		}
		var err error
		diff, err = applyDiff(bucket, onDisk, encoded, next)
		after = gen.Add(1)
		return err
	})
	if err != nil {
		return Diff{}, err
	}
	m.written, m.gen, m.known = next, after, true
	return diff, nil
}

// encode validates and encodes items and digests each value.
func (m *Mirror[T]) encode(
	items map[string]Item[T],
) (map[string][]byte, map[string]digest, error) {
	now := m.keyed.store.now()
	encoded := make(map[string][]byte, len(items))
	next := make(map[string]digest, len(items))
	for key, item := range items {
		if key == "" {
			return nil, nil, ErrEmptyKey
		}
		data, err := encode(item.Value, now, item.Expires)
		if err != nil {
			return nil, nil, err
		}
		encoded[key], next[key] = data, digestOf(data)
	}
	return encoded, next, nil
}

// unchanged reports whether next holds exactly the digests of written.
func unchanged(written, next map[string]digest) bool {
	if len(written) != len(next) {
		return false
	}
	for key, d := range next {
		if old, ok := written[key]; !ok || old != d {
			return false
		}
	}
	return true
}

// digestsOf reads the digest of every value in bucket.
func digestsOf(bucket *bolt.Bucket) map[string]digest {
	out := map[string]digest{}
	cursor := bucket.Cursor()
	for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
		// A short value is corrupt: keep its key with the zero digest so
		// applyDiff rewrites or deletes it instead of leaving it behind.
		if len(v) >= headerSize {
			out[string(k)] = digestOf(v)
		} else {
			out[string(k)] = digest{}
		}
	}
	return out
}

// applyDiff writes the keys of encoded whose digest differs from onDisk
// and deletes the keys of onDisk that next does not hold.
func applyDiff(
	bucket *bolt.Bucket, onDisk map[string]digest,
	encoded map[string][]byte, next map[string]digest,
) (Diff, error) {
	var diff Diff
	for key := range onDisk {
		if _, keep := next[key]; keep {
			continue
		}
		if err := bucket.Delete([]byte(key)); err != nil {
			return diff, err
		}
		diff.Deleted++
	}
	for key, d := range next {
		if old, ok := onDisk[key]; ok && old == d {
			continue
		}
		if err := bucket.Put([]byte(key), encoded[key]); err != nil {
			return diff, err
		}
		diff.Put++
	}
	return diff, nil
}

// generation returns the write counter of bucket b. Every keyed write
// transaction bumps it, so a Mirror can tell whether someone else wrote.
func (f *file) generation(b Bucket) *atomic.Uint64 {
	return f.writes[b]
}

// touch records a write to bucket b. Call it inside the transaction.
func (f *file) touch(b Bucket) {
	f.writes[b].Add(1)
}

func newGenerations() map[Bucket]*atomic.Uint64 {
	out := make(map[Bucket]*atomic.Uint64, len(specs))
	for _, sp := range specs {
		out[sp.name] = new(atomic.Uint64)
	}
	return out
}
