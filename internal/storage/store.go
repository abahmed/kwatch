package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

// SchemaVersion is the on-disk format version. Any change to a persisted
// layout must bump it. There are no migrations: a file
// with another version is moved aside and replaced by a fresh store (see
// reset.go).
const SchemaVersion = 2

// Errors returned by the store.
var (
	// ErrFenced means a later writer has claimed the store, or this
	// handle was abandoned by a newer claim in this process.
	ErrFenced = errors.New("store claimed by a newer lease holder")
	// ErrNotClaimed means a write was attempted before Claim.
	ErrNotClaimed = errors.New("store not claimed")
	// ErrAlreadyClaimed means Claim was called twice on one handle. A
	// new leadership term takes a new handle with ClaimNew.
	ErrAlreadyClaimed = errors.New("store handle already claimed")
	// ErrRepairPending means the file is unusable and will be reset by
	// the first Claim; until then nothing can be read.
	ErrRepairPending = errors.New("store reset pending until claim")
)

var (
	metaBucket  = []byte("meta")
	versionKey  = []byte("schema.version")
	epochKey    = []byte("lease.epoch")
	openTimeout = 5 * time.Second
)

// Options configures Open.
type Options struct {
	// Now is the clock used for record timestamps, expiry and
	// retention. It is required.
	Now func() time.Time
	// SizeCap is the logical size limit the compactor enforces. Open
	// copy-compacts a file that grew a quarter past it (shrink.go). Zero
	// means DefaultSizeCap.
	SizeCap int64
	// FreeSpace reports the free bytes on the volume holding a path. It
	// guards the startup copy; nil uses the operating system.
	FreeSpace func(path string) (uint64, error)
	// VolumeLimit is the real size limit of the volume holding the
	// file, when it is smaller than what the file system reports. An
	// emptyDir with a sizeLimit reports the node's disk, but the kubelet
	// evicts the Pod once the volume passes its sizeLimit; pass that
	// limit here. The free space for the startup copy is then at most
	// VolumeLimit minus the bytes already in the file's directory. Zero
	// trusts FreeSpace alone.
	VolumeLimit int64
	// DeferRepair opens the file in inspect state: nothing is written
	// before the first Claim, which the caller makes only once it holds
	// the Lease. The file is opened read-only (a missing file is not
	// created, and reads find nothing), an unusable file is not
	// deleted, and an oversized one is not rewritten until Claim.
	// Pending reports what Claim will do. False repairs during Open.
	DeferRepair bool
}

// Store is a handle on an open state file. Handles from Open start
// unclaimed: they can read but not write. Claim binds a handle to one
// claim epoch for its whole life.
type Store struct {
	*file
	epoch     atomic.Uint64
	abandoned atomic.Bool
}

// file is what every handle of one open state file shares.
type file struct {
	path    string
	now     func() time.Time
	sizeCap int64
	free    freeSpaceFunc
	// volumeLimit is Options.VolumeLimit; zero means no volume budget.
	volumeLimit int64

	mu      sync.RWMutex
	db      *bolt.DB       // nil while a reset is pending or absent
	absent  bool           // no file yet; Claim creates it
	inspect bool           // db is read-only until Claim
	reset   *Reset         // the reset Open or Claim performed
	pending *unusableError // a reset deferred until Claim
	shrink  bool           // a rewrite deferred until Claim
	latest  *Store         // the newest claimed handle
	closers []func()       // run once by Close, before the file closes

	writes   map[Bucket]*atomic.Uint64 // see generation in mirror.go
	counters counters
}

// Open opens or creates the state file at path. The directory is created
// with owner-only permissions; the file is 0600. A file with another
// schema version, or one bbolt cannot read, is deleted and a fresh
// store is created without a backup; Reset reports it. With
// Options.DeferRepair the deletion and the oversize rewrite wait for
// Claim. Open fails only when the file cannot be opened at all, for
// example while another process holds it.
func Open(path string, options Options) (*Store, error) {
	if options.Now == nil {
		return nil, errors.New("store: clock is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create directory: %w", err)
	}
	f := &file{
		path: path, now: options.Now,
		sizeCap: capOrDefault(options.SizeCap), free: freeSpaceOf(options),
		volumeLimit: options.VolumeLimit, writes: newGenerations(),
	}
	var db *bolt.DB
	var err error
	if options.DeferRepair {
		db, f.absent, err = openForInspection(path)
	} else {
		db, err = openChecked(path)
	}
	var bad *unusableError
	if err != nil && !errors.As(err, &bad) {
		return nil, err
	}
	f.db, f.pending = db, bad
	f.inspect = options.DeferRepair && db != nil
	f.shrink = db != nil && oversized(path, f.sizeCap)
	if !options.DeferRepair {
		if err := f.repair(); err != nil {
			return nil, err
		}
	}
	return &Store{file: f}, nil
}

// repair performs the pending reset and rewrite. It runs once, under
// the file lock, before anything is written.
func (f *file) repair() error {
	if err := f.openForWriting(); err != nil {
		return err
	}
	if f.pending != nil {
		db, err := resetFile(f.path, f.pending, f.volumeLimit)
		if err != nil {
			return err
		}
		f.db, f.reset, f.pending = db, &Reset{
			Reason: f.pending.reason, OldVersion: f.pending.version,
		}, nil
	}
	if f.shrink {
		db, err := compactOversized(f.db, f.path, f.sizeCap, f.free)
		if err != nil {
			f.db = nil
			return err
		}
		f.db, f.shrink = db, false
	}
	return nil
}

// openForWriting replaces the read-only handle of a deferred Open with
// a writable one, creating the file and its buckets when needed.
func (f *file) openForWriting() error {
	if !f.absent && !f.inspect {
		return nil
	}
	if f.db != nil {
		if err := f.db.Close(); err != nil {
			return fmt.Errorf("store: close inspected file: %w", err)
		}
		f.db = nil
	}
	open := openWritable
	if f.absent {
		open = openChecked
	}
	db, err := open(f.path)
	var bad *unusableError
	if err != nil && !errors.As(err, &bad) {
		return err
	}
	f.db, f.absent, f.inspect = db, false, false
	if bad != nil {
		f.pending = bad
	}
	return nil
}

// Close runs the OnClose functions, then releases the file for every
// handle of it.
func (s *Store) Close() error {
	s.mu.Lock()
	closers := s.closers
	s.closers = nil
	s.mu.Unlock()
	for _, fn := range closers {
		fn()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// OnClose registers fn to run once when the file is closed, while it
// can still be written. A view that holds back writes, such as the
// pipeline's fingerprints, uses it for its final write. fn must be
// short: Close waits for it.
func (s *Store) OnClose(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closers = append(s.closers, fn)
}

// Reset reports whether Open or Claim replaced an unusable file, and how.
func (s *Store) Reset() (Reset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.reset == nil {
		return Reset{}, false
	}
	return *s.reset, true
}

// Pending reports the reset that the first Claim will perform on a file
// opened with Options.DeferRepair.
func (s *Store) Pending() (Reset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pending == nil {
		return Reset{}, false
	}
	return Reset{
		Reason: s.pending.reason, OldVersion: s.pending.version,
	}, true
}

// Now returns the store clock's current time.
func (s *Store) Now() time.Time {
	return s.now()
}

// Claim binds this unclaimed handle to a new epoch and returns it. It
// first performs any repair that Open deferred. The epoch is a
// store-local counter: every claim stores the previous value plus one,
// so a claim always succeeds and supersedes earlier holders, in this
// process (whose handles are abandoned at once) and in any other (whose
// next write sees the newer stored epoch). The caller must hold the
// leader Lease; the counter never depends on Lease fields, so a deleted
// or renamed Lease cannot make the file unclaimable.
func (s *Store) Claim() (uint64, error) {
	if s.epoch.Load() != 0 {
		return 0, ErrAlreadyClaimed
	}
	return s.claimInto(s)
}

// ClaimNew returns a new handle on the same file bound to a new epoch,
// for a leadership term that reuses an open file. Every earlier handle
// is abandoned: goroutines still holding one get ErrFenced.
func (s *Store) ClaimNew() (*Store, error) {
	next := &Store{file: s.file}
	if _, err := s.claimInto(next); err != nil {
		return nil, err
	}
	return next, nil
}

func (f *file) claimInto(handle *Store) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.repair(); err != nil {
		return 0, err
	}
	var epoch uint64
	err := f.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		epoch = readUint(meta.Get(epochKey)) + 1
		return meta.Put(epochKey, writeUint(epoch))
	})
	if err != nil {
		return 0, fmt.Errorf("store: claim: %w", err)
	}
	if f.latest != nil && f.latest != handle {
		f.latest.abandoned.Store(true)
	}
	f.latest = handle
	handle.epoch.Store(epoch)
	return epoch, nil
}

// Epoch returns the epoch this handle claimed, or zero before Claim.
func (s *Store) Epoch() uint64 {
	return s.epoch.Load()
}

// Abandoned reports whether a newer claim in this process replaced
// this handle.
func (s *Store) Abandoned() bool {
	return s.abandoned.Load()
}

// update runs fn in a write transaction after verifying that this handle
// still holds the newest claim. Every write, including compaction, goes
// through it, so a deposed leader can neither write nor delete.
func (s *Store) update(fn func(*bolt.Tx) error) error {
	epoch := s.epoch.Load()
	if epoch == 0 {
		return ErrNotClaimed
	}
	if s.abandoned.Load() {
		return ErrFenced
	}
	s.counters.writeTxs.Add(1)
	return s.bolt().Update(func(tx *bolt.Tx) error {
		stored := readUint(tx.Bucket(metaBucket).Get(epochKey))
		if stored != epoch {
			return ErrFenced
		}
		return fn(tx)
	})
}

// view runs fn in a read transaction. Before the first Claim of a
// deferred Open with no file yet there is nothing to read: fn is not
// called and view returns nil, as for an empty store.
func (s *Store) view(fn func(*bolt.Tx) error) error {
	db, absent := s.readable()
	if db == nil {
		if absent {
			return nil
		}
		return ErrRepairPending
	}
	return db.View(fn)
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
