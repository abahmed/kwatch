package storage

import (
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
//   - It runs when the free pages are a large part of the file, or when
//     the file is at least twice the data it holds. Keys written in no
//     particular order leave pages about half full, and bbolt counts
//     them as used, not free: such a file never looks free enough, yet
//     a copy with full pages is about half its size. The file is mapped
//     into memory, so its size, not its data, is what reading it costs
//     in resident memory.
//   - It copies the live data next to the file, as the startup rewrite
//     does (shrink.go), and renames the copy over the original. Writes
//     wait on the file gate during the copy, which takes about a second
//     per 50 MiB of live data; reads continue until the swap. The
//     decision loop never writes to the file itself, so it does not wait.
//   - The copy is the crash-safe part: the original stays whole until
//     the rename, and a copy a crash left behind is removed by the next
//     claim (removeLeftoverCopy). The copy is opened before the rename,
//     so the store always has a database: when the copy cannot be
//     opened or renamed, the original stays in use.
//   - A copy only counts when it made the file smaller. A file can be
//     twice its data even when its pages are full (records of about one
//     page each), so after a copy the next rewrite waits until the file
//     is again sparseFactor times the size that copy left; a copy that
//     gained nothing is logged once.

// reclaimMin is the free space inside the file below which a rewrite is
// not worth its pause. It is a variable so tests can use small files.
var reclaimMin int64 = 64 << 20

// sparseFactor is how many times its data a file may be before its
// half-full pages are worth a rewrite.
const sparseFactor = 2

// reclaimDue reports whether a file of size bytes is worth a rewrite:
// its free pages are at least reclaimMin and half of it, or it is at
// least sparseFactor times its logical data (0: not measured yet) and
// that excess is at least reclaimMin. lastCopy, the size the previous
// rewrite left (0: none), must be outgrown sparseFactor times too.
func reclaimDue(size, free, logical, lastCopy int64) bool {
	if free >= reclaimMin && free >= size/2 {
		return true
	}
	return logical > 0 && size >= sparseFactor*logical &&
		size-logical >= reclaimMin && size >= sparseFactor*lastCopy
}

// openCopy opens a rewrite's copy; a variable so tests can fail it.
var openCopy = func(path string) (*bolt.DB, error) {
	return bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
}

// removeLeftoverCopy deletes a rewrite's copy a crash left next to the
// file at path, if any.
func removeLeftoverCopy(path string) {
	tmp := path + ".compact"
	if err := os.Remove(tmp); err == nil {
		klog.InfoS("removed a state file copy a crash left behind",
			"component", "state", "operation", "reclaim", "path", tmp)
	}
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
	if s.retry.waiting(s.now()) {
		return false, nil
	}
	size, free := s.fileUsage()
	if !reclaimDue(size, free, s.counters.logicalBytes.Load(),
		s.lastCopy) {
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
// gate, so nothing writes. The copy is opened before the rename: its
// file follows the rename, so the store never lacks a database. The old
// database closes after the swap, outside mu, once its running readers
// finish; readers that start meanwhile use the copy.
func (f *file) swapInCopy(db *bolt.DB) (bool, error) {
	rewriting := startStep("rewrite")
	tmp := f.path + ".compact"
	before, _ := fileSize(f.path)
	if err := copyTo(db, tmp); err != nil {
		klog.ErrorS(err, "state file rewrite failed; using it as is",
			"component", "state", "operation", "reclaim")
		f.retry.fail(f.now())
		return false, nil
	}
	after, _ := fileSize(tmp)
	if after >= before {
		_ = os.Remove(tmp)
		f.lastCopy = after
		noteCopy(db, after)
		if !f.copyNoGain {
			f.copyNoGain = true
			klog.InfoS("state file rewrite would not make it smaller; "+
				"waiting until it grows", "component", "state",
				"operation", "reclaim", "fileBytes", before,
				"copyBytes", after)
		}
		return false, nil
	}
	copied, err := openCopy(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		klog.ErrorS(err, "state file rewrite could not open its copy; "+
			"using the file as is", "component", "state",
			"operation", "reclaim")
		f.retry.fail(f.now())
		return false, nil
	}
	old, swapped := f.swap(copied, tmp)
	if !swapped {
		f.retry.fail(f.now())
		return false, nil
	}
	if err := old.Close(); err != nil {
		klog.ErrorS(err, "closing the state file after a rewrite",
			"component", "state", "operation", "reclaim")
	}
	// Only a copy that went live sets the size the next rewrite waits
	// for; a failed open or swap must not delay the retry.
	f.lastCopy = after
	noteCopy(copied, after)
	f.copyNoGain = false
	f.retry.reset()
	f.counters.rewrites.Add(1)
	klog.InfoS("rewrote state file while running", "component", "state",
		"operation", "reclaim", "beforeBytes", before, "afterBytes", after,
		"durationMs", rewriting.end().Milliseconds())
	return true, nil
}

// swap renames the open copy over the file and makes it the database,
// returning the one it replaced. When the store closed meanwhile or the
// rename fails, the copy is dropped and nothing changes.
func (f *file) swap(copied *bolt.DB, tmp string) (*bolt.DB, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		_ = copied.Close()
		_ = os.Remove(tmp)
		return nil, false
	}
	if err := os.Rename(tmp, f.path); err != nil {
		klog.ErrorS(err, "state file swap failed; using it as is",
			"component", "state", "operation", "reclaim")
		_ = copied.Close()
		_ = os.Remove(tmp)
		return nil, false
	}
	_ = syncDir(filepath.Dir(f.path))
	old := f.db
	f.db = copied
	return old, true
}

// noteCopy keeps the size a rewrite left (lastCopy) in db, so a file
// that can't shrink is not copied again after every restart. A failed
// write is only logged: the next rewrite measures again.
func noteCopy(db *bolt.DB, size int64) {
	err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(lastCopyKey,
			writeUint(uint64(size)))
	})
	if err != nil {
		klog.ErrorS(err, "state file rewrite size not kept",
			"component", "state", "operation", "reclaim")
	}
}
