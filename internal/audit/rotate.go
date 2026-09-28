package audit

import (
	"os"
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
}

func openRotatingFile(path string, maxBytes int64) (*rotatingFile, error) {
	r := &rotatingFile{path: path, maxBytes: maxBytes}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	// #nosec G304 -- operator-selected audit path
	f, err := os.OpenFile(
		r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600,
	)
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
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) rotate() error {
	if err := r.file.Close(); err != nil {
		return err
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return err
	}
	return r.open()
}

func (r *rotatingFile) Close() error {
	return r.file.Close()
}
