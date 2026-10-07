package storage

import (
	"errors"

	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
)

// reopenTries is how often view looks for the database again when an
// online rewrite swapped it between looking it up and starting.
const reopenTries = 3

// update runs fn in a write transaction after verifying that this handle
// still holds the newest claim. Every write, including compaction, goes
// through it, so a deposed leader can neither write nor delete. It holds
// the file gate shared, so a rewrite or a claim never overlaps a write.
func (s *Store) update(fn func(*bolt.Tx) error) error {
	epoch := s.epoch.Load()
	if epoch == 0 {
		return ErrNotClaimed
	}
	if s.abandoned.Load() {
		return ErrFenced
	}
	s.gate.RLock()
	defer s.gate.RUnlock()
	db := s.bolt()
	if db == nil {
		return ErrRepairPending
	}
	s.counters.writeTxs.Add(1)
	return db.Update(func(tx *bolt.Tx) error {
		stored := readUint(tx.Bucket(metaBucket).Get(epochKey))
		if stored != epoch {
			return ErrFenced
		}
		return fn(tx)
	})
}

// view runs fn in a read transaction. Before the first Claim of a
// deferred Open with no file yet there is nothing to read: fn is not
// called and view returns nil, as for an empty store. fn must not start
// another transaction on this file: an online rewrite waits for open
// readers, and a second reader would wait for that rewrite.
func (s *Store) view(fn func(*bolt.Tx) error) error {
	for try := 1; ; try++ {
		db, absent := s.readable()
		if db == nil {
			if absent {
				return nil
			}
			return ErrRepairPending
		}
		err := db.View(fn)
		if errors.Is(err, berrors.ErrDatabaseNotOpen) && try < reopenTries {
			continue // swapped by a rewrite; the next look finds the new one
		}
		return err
	}
}

func (f *file) readable() (*bolt.DB, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.db, f.absent
}

// bolt returns the open database; nil while a reset is pending.
func (f *file) bolt() *bolt.DB {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.db
}
