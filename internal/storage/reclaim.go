package storage

import (
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
	"k8s.io/klog/v2"
)

// bbolt keeps freed pages inside the file. The compactor deletes expired
// and surplus entries every pass, which frees pages that later writes
// reuse, so a file whose content is bounded stops growing. It does not
// shrink, though: after a burst (or after an older version that kept
// more) most of the file can be free pages, and every start then pays to
// check them as part of the file. Reclaim gives that space back while
// running, without waiting for a restart:
//
//   - It runs when the free pages are a large part of the file.
//   - It copies the live data next to the file, as the startup rewrite
//     does (shrink.go), and renames the copy over the original. Writes
//     wait on the file gate during the copy, which takes about a second
//     per 50 MiB of live data; reads continue until the swap. The
//     decision loop never writes to the file itself, so it does not wait.
//   - The copy is the crash-safe part: the original stays whole until
//     the rename, and a leftover copy is removed by the next rewrite.

// reclaimMin is the free space inside the file below which a rewrite is
// not worth its pause. It is a variable so tests can use small files.
var reclaimMin int64 = 64 << 20

// reclaimDue reports whether free is at least reclaimMin and at least
// half of a file of size bytes.
func reclaimDue(size, free int64) bool {
	return free >= reclaimMin && free >= size/2
}

// fileUsage returns the size of the state file and how much of it is
// free pages that writes can reuse.
func (s *Store) fileUsage() (size, free int64) {
	size, _ = fileSize(s.path)
	if db := s.bolt(); db != nil {
		free = int64(db.Stats().FreeAlloc)
	}
	return size, free
}

// Reclaim rewrites the file when most of it is free pages, and reports
// whether it did. It fails with ErrFenced when a newer holder claimed
// the file. A copy that cannot be made, for lack of space or any other
// reason, is logged and leaves the file as it was.
func (s *Store) Reclaim() (bool, error) {
	epoch := s.epoch.Load()
	if epoch == 0 {
		return false, ErrNotClaimed
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	if s.abandoned.Load() {
		return false, ErrFenced
	}
	db := s.bolt()
	if db == nil {
		return false, ErrRepairPending
	}
	if size, free := s.fileUsage(); !reclaimDue(size, free) {
		return false, nil
	}
	err := db.View(func(tx *bolt.Tx) error {
		if readUint(tx.Bucket(metaBucket).Get(epochKey)) != epoch {
			return ErrFenced
		}
		return nil
	})
	if err != nil || !hasRoomToCopy(db, s.path, s.free) {
		return false, err
	}
	return s.swapInCopy(db)
}

// swapInCopy copies db and replaces it by the copy. The caller holds the
// gate, so nothing writes. Readers wait on the file lock for the swap;
// the old database closes once its running readers finish.
func (f *file) swapInCopy(db *bolt.DB) (bool, error) {
	rewriting := startStep("rewrite")
	tmp := f.path + ".compact"
	before, _ := fileSize(f.path)
	if err := copyTo(db, tmp); err != nil {
		klog.ErrorS(err, "state file rewrite failed; using it as is",
			"component", "state", "operation", "reclaim")
		return false, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		_ = os.Remove(tmp)
		return false, nil
	}
	if err := db.Close(); err != nil {
		f.db = nil
		return false, fmt.Errorf("store: close before swap: %w", err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		klog.ErrorS(err, "state file swap failed; using it as is",
			"component", "state", "operation", "reclaim")
		_ = os.Remove(tmp)
	} else {
		_ = syncDir(filepath.Dir(f.path))
	}
	reopened, err := bolt.Open(f.path, 0o600,
		&bolt.Options{Timeout: openTimeout})
	if err != nil {
		f.db = nil
		return false, fmt.Errorf("store: reopen after rewrite: %w", err)
	}
	f.db = reopened
	after, _ := fileSize(f.path)
	if after >= before {
		return false, nil
	}
	f.counters.rewrites.Add(1)
	klog.InfoS("rewrote state file while running", "component", "state",
		"operation", "reclaim", "beforeBytes", before, "afterBytes", after,
		"durationMs", rewriting.end().Milliseconds())
	return true, nil
}
