package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// SchemaVersion is the on-disk format version. A change to any persisted
// layout must bump it and add a migration in migrate.go.
const SchemaVersion = 1

// Errors returned by the store.
var (
	// ErrFenced means a newer Lease holder has claimed the store.
	ErrFenced = errors.New("store claimed by a newer lease holder")
	// ErrNotClaimed means a write was attempted before Claim.
	ErrNotClaimed = errors.New("store not claimed")
	// ErrNewerSchema means the file was written by a newer kwatch.
	ErrNewerSchema = errors.New("store schema is newer than this kwatch")
)

var (
	metaBucket  = []byte("meta")
	versionKey  = []byte("schema.version")
	epochKey    = []byte("lease.epoch")
	openTimeout = 5 * time.Second
)

// Options configures Open.
type Options struct {
	// Now is the clock used for record timestamps and retention.
	Now func() time.Time
}

// Store is an open state file.
type Store struct {
	db    *bolt.DB
	now   func() time.Time
	epoch uint64
}

// Open opens or creates the state file at path. The directory is created
// with owner-only permissions; the file is 0600.
func Open(path string, options Options) (*Store, error) {
	if options.Now == nil {
		return nil, errors.New("store: clock is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create directory: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	s := &Store{db: db, now: options.Now}
	if err := s.initialise(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the file.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initialise() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		version := readUint(meta.Get(versionKey))
		if version > SchemaVersion {
			return fmt.Errorf("%w: file %d, supported %d",
				ErrNewerSchema, version, SchemaVersion)
		}
		if err := migrate(tx, version); err != nil {
			return fmt.Errorf("store: migrate from %d: %w", version, err)
		}
		return meta.Put(versionKey, writeUint(SchemaVersion))
	})
}

// Claim records epoch as the current writer. It fails with ErrFenced when
// a newer epoch already claimed the store.
func (s *Store) Claim(epoch uint64) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if stored := readUint(meta.Get(epochKey)); stored > epoch {
			return ErrFenced
		}
		return meta.Put(epochKey, writeUint(epoch))
	})
	if err == nil {
		s.epoch = epoch
	}
	return err
}

// update runs fn in a write transaction after verifying that this store
// still holds the newest claim.
func (s *Store) update(fn func(*bolt.Tx) error) error {
	if s.epoch == 0 {
		return ErrNotClaimed
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		stored := readUint(tx.Bucket(metaBucket).Get(epochKey))
		if stored != s.epoch {
			return ErrFenced
		}
		return fn(tx)
	})
}

func (s *Store) view(fn func(*bolt.Tx) error) error {
	return s.db.View(fn)
}

func readUint(data []byte) uint64 {
	if len(data) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}

func writeUint(value uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, value)
	return out
}
