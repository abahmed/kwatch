package storage

import (
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
	"k8s.io/klog/v2"
)

// bbolt reuses freed pages but never returns them to the file system, so
// the compactor keeps the logical size under the cap while the file can
// stay larger. Open therefore rewrites a file that exceeds the cap by
// a margin: it copies live data into state.db.compact and renames
// the copy over the original. This runs before the first claim (in
// Open, or in Claim with Options.DeferRepair), so no writer can see a
// half-made copy.
// It needs free disk space for the live data plus a margin; without it the
// rewrite is skipped and logged. On any failure the original file is kept
// unchanged.

// compactMarginDivisor sets the margin: the file must exceed the cap by
// a quarter of the cap before Open rewrites it.
const compactMarginDivisor = 4

// compactTxBytes bounds each copy transaction.
const compactTxBytes = 64 << 20

// compactSpaceMargin is the free space required beyond the live data.
const compactSpaceMargin = 64 << 20

// freeSpaceFunc reports free bytes on the volume holding a path.
type freeSpaceFunc func(path string) (uint64, error)

// hasRoomToCopy reports whether the volume can hold a copy of the live
// data plus the margin. Unknown free space counts as not enough.
func hasRoomToCopy(db *bolt.DB, path string, free freeSpaceFunc) bool {
	stats := db.Stats()
	live := uint64(0)
	if size, err := fileSize(path); err == nil &&
		size > int64(stats.FreeAlloc) {
		live = uint64(size) - uint64(stats.FreeAlloc)
	}
	have, err := free(path)
	if err != nil {
		klog.ErrorS(err, "state file rewrite skipped: free space unknown",
			"component", "state", "operation", "compact")
		return false
	}
	if have < live+compactSpaceMargin {
		klog.InfoS("state file rewrite skipped: not enough free space",
			"component", "state", "operation", "compact",
			"liveBytes", live, "freeBytes", have)
		return false
	}
	return true
}

// oversized reports whether the file at path exceeds sizeCap by the
// rewrite margin.
func oversized(path string, sizeCap int64) bool {
	size, err := fileSize(path)
	return err == nil && size > sizeCap+sizeCap/compactMarginDivisor
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// compactOversized returns db, or a reopened rewritten copy when the file
// is larger than sizeCap plus the margin. A failed copy is logged and the
// original file is used; Open fails only if the file cannot be reopened
// after the swap, which leaves it intact for the next start.
func compactOversized(
	db *bolt.DB, path string, sizeCap int64, free freeSpaceFunc,
) (*bolt.DB, error) {
	if !oversized(path, sizeCap) {
		return db, nil
	}
	size, _ := fileSize(path)
	if !hasRoomToCopy(db, path, free) {
		return db, nil
	}
	if err := copyTo(db, path+".compact"); err != nil {
		klog.ErrorS(err, "state file rewrite failed; using it as is",
			"component", "state", "operation", "compact")
		return db, nil
	}
	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("store: close before swap: %w", err)
	}
	if err := os.Rename(path+".compact", path); err != nil {
		klog.ErrorS(err, "state file swap failed; using it as is",
			"component", "state", "operation", "compact")
		_ = os.Remove(path + ".compact")
	} else {
		_ = syncDir(filepath.Dir(path))
		klog.InfoS("rewrote oversized state file", "component", "state",
			"operation", "compact", "beforeBytes", size)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return nil, fmt.Errorf("store: reopen after compact: %w", err)
	}
	return db, nil
}

// copyTo writes the live data of db into a new file at tmp. On error tmp
// is removed and db is untouched.
func copyTo(db *bolt.DB, tmp string) error {
	_ = os.Remove(tmp)
	dst, err := bolt.Open(tmp, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return err
	}
	err = bolt.Compact(dst, db, compactTxBytes)
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
