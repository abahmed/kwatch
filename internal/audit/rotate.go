package audit

import (
	"errors"
	"os"
	"time"

	"k8s.io/klog/v2"
)

// maxAuditFileBytes bounds the audit file. The container filesystem is
// usually small and ephemeral, so one rotated backup is kept.
const maxAuditFileBytes = 10 << 20

// rotatingFile appends to path and renames it to path+".1" once it would
// exceed maxBytes. Callers serialize writes.
type rotatingFile struct {
	path     string
	maxBytes int64
	file     *os.File
	size     int64

	// detached is true when a rotation renamed the file but reopening
	// the path failed: file then points at path+".1". Writes continue
	// there only up to detachedCap bytes, and the reopen is retried at
	// most once per reopenBackoff.
	detached bool
	retryAt  time.Time
	// rotateRetryAt is when a rotation whose rename failed may be tried
	// again. Until then writes just append, so a rename that keeps
	// failing logs once per reopenBackoff and not once per entry.
	rotateRetryAt time.Time
	now           func() time.Time
	openFile      func(path string) (*os.File, error)
}

// reopenBackoff is the pause between attempts to reopen the audit path
// after a rotation lost it.
const reopenBackoff = 30 * time.Second

// detachedCapFactor times maxBytes is the most a renamed file may hold
// while the audit path cannot be reopened.
const detachedCapFactor = 2

// errAuditDropped is returned for an entry dropped because the audit file
// cannot be reopened and the renamed file reached its size bound.
var errAuditDropped = errors.New("audit entry dropped: log file unavailable")

func openRotatingFile(path string, maxBytes int64) (*rotatingFile, error) {
	r := &rotatingFile{
		path: path, maxBytes: maxBytes,
		now: time.Now, openFile: openAuditFile,
	}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func openAuditFile(path string) (*os.File, error) {
	// #nosec G304 -- operator-selected audit path
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
}

func (r *rotatingFile) open() error {
	f, err := r.openFile(r.path)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.file, r.size = f, info.Size()
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	if r.detached {
		return r.writeDetached(p)
	}
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes &&
		!r.now().Before(r.rotateRetryAt) {
		// A failed rotation must not lose the entry; keep appending.
		if err := r.rotate(); err != nil {
			klog.ErrorS(err, "failed to rotate audit log",
				"component", "audit", "operation", "rotate")
			r.rotateRetryAt = r.now().Add(reopenBackoff)
		}
		if r.detached {
			return r.writeDetached(p)
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// writeDetached handles a write while r.file is the renamed backup. It
// retries the reopen after the backoff and otherwise appends to the
// backup only while it stays under the size bound.
func (r *rotatingFile) writeDetached(p []byte) (int, error) {
	if !r.now().Before(r.retryAt) {
		if err := r.reattach(); err != nil {
			r.retryAt = r.now().Add(reopenBackoff)
			klog.ErrorS(err, "audit log path is still unavailable",
				"component", "audit", "operation", "reopen")
		} else {
			n, err := r.file.Write(p)
			r.size += int64(n)
			return n, err
		}
	}
	if r.size+int64(len(p)) > r.maxBytes*detachedCapFactor {
		return 0, errAuditDropped
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// reattach reopens the audit path and drops the renamed backup handle.
// Once the path is open, r.file is the new file and the rotation worked;
// a failure to close the old handle changes nothing about that, so it is
// only logged.
func (r *rotatingFile) reattach() error {
	old := r.file
	if err := r.open(); err != nil {
		return err
	}
	r.detached = false
	if err := old.Close(); err != nil {
		klog.ErrorS(err, "failed to close the rotated audit log",
			"component", "audit", "operation", "close")
	}
	return nil
}

// rotate keeps r.file a writable handle whether it succeeds or fails: the
// old file is closed only after the new one is open, and a rename on an open
// file is safe, so a failed step leaves the previous handle in use. When
// only the reopen fails the path is gone and r.file is the backup; the
// file is then detached and Write bounds and retries (see writeDetached).
func (r *rotatingFile) rotate() error {
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return err
	}
	if err := r.reattach(); err != nil {
		r.detached = true
		r.retryAt = r.now().Add(reopenBackoff)
		return err
	}
	return nil
}

func (r *rotatingFile) Close() error {
	return r.file.Close()
}
