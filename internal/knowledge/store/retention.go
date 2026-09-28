package store

import (
	"time"

	bolt "go.etcd.io/bbolt"
)

// compactBatch bounds one retention transaction so compaction never holds
// the write lock long enough to delay the hot path.
const compactBatch = 1000

// Compact deletes values in c written before cutoff. It works in bounded
// batches and returns how many values it removed.
func (s *Store) Compact(c Collection, cutoff time.Time) (int, error) {
	total := 0
	for {
		removed := 0
		err := s.update(func(tx *bolt.Tx) error {
			bucket, err := bucketFor(tx, c)
			if err != nil {
				return err
			}
			var expired [][]byte
			cursor := bucket.Cursor()
			for k, v := cursor.First(); k != nil &&
				len(expired) < compactBatch; k, v = cursor.Next() {
				if writtenAt(v).Before(cutoff) {
					expired = append(expired, append([]byte(nil), k...))
				}
			}
			for _, key := range expired {
				if err := bucket.Delete(key); err != nil {
					return err
				}
			}
			removed = len(expired)
			return nil
		})
		total += removed
		if err != nil || removed < compactBatch {
			return total, err
		}
	}
}

// Size returns the state file size in bytes, for the size cap and health.
func (s *Store) Size() (int64, error) {
	var size int64
	err := s.view(func(tx *bolt.Tx) error {
		size = tx.Size()
		return nil
	})
	return size, err
}
