package storage

import (
	"io/fs"
	"path/filepath"

	"k8s.io/klog/v2"
)

// The compactor's caps count logical bytes, but the volume fills with
// physical ones: bbolt keeps freed pages inside the file, and the
// kubelet evicts a Pod whose emptyDir passes its sizeLimit. The cap
// therefore also applies to the file size:
//
//   - When the file is clearly larger than SizeCap, the pass lowers its
//     logical target by the excess. The freed pages are then reused for
//     new writes, so the file stops growing.
//   - When the file passes SizeCap by the rewrite margin, the pass
//     reports RewriteDue and the next Open (or Claim) rewrites the file
//     with copy-compaction (shrink.go). A rewrite while running would
//     pause every reader and writer for the whole copy.
//   - The rewrite checks free space against the volume's real limit
//     (Options.VolumeLimit), not only what the file system reports.

// fileOverhead is file size that says nothing about growth: bbolt needs
// pages of its own (meta, freelist, branch pages) however small the data.
const fileOverhead = 1 << 20

// physicalTarget is the logical size the total cap aims for when the
// file holds fileBytes. A file within SizeCap plus a quarter (or
// fileOverhead, whichever is larger) leaves the target at SizeCap; each
// byte beyond lowers it by one byte.
func physicalTarget(sizeCap, fileBytes int64) int64 {
	allowed := sizeCap + max(sizeCap/compactMarginDivisor, fileOverhead)
	return sizeCap - max(fileBytes-allowed, 0)
}

// noteRewriteDue records whether the file needs a startup rewrite and
// logs once each time it starts to.
func (s *Store) noteRewriteDue(due bool, fileBytes int64) {
	if s.counters.rewriteDue.Swap(due) || !due {
		return
	}
	klog.InfoS("state file over its size cap; it is rewritten at the "+
		"next start", "component", "state", "operation", "compact",
		"fileBytes", fileBytes, "sizeCapBytes", s.sizeCap)
}

// freeSpaceOf returns the free space function Open uses: the configured
// or operating system one, bounded by Options.VolumeLimit.
func freeSpaceOf(options Options) freeSpaceFunc {
	free := options.FreeSpace
	if free == nil {
		free = osFreeSpace
	}
	limit := options.VolumeLimit
	if limit <= 0 {
		return free
	}
	return func(path string) (uint64, error) {
		have, err := free(path)
		if err != nil {
			return 0, err
		}
		used, err := dirBytes(filepath.Dir(path))
		if err != nil {
			return 0, err
		}
		return min(have, uint64(max(limit-used, 0))), nil
	}
}

// dirBytes is the size of every regular file under dir: what the
// kubelet counts against an emptyDir's sizeLimit.
func dirBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(
		_ string, d fs.DirEntry, err error,
	) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
