package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
	"k8s.io/klog/v2"
)

// Reset reasons. They are bounded values safe for health and metrics.
const (
	// ResetSchemaMismatch means the file had another (older or newer)
	// schema version.
	ResetSchemaMismatch = "schema_mismatch"
	// ResetUnreadable means bbolt could not open or read the file.
	ResetUnreadable = "unreadable"
)

// Reset describes a state file that Open replaced.
type Reset struct {
	// Reason is ResetSchemaMismatch or ResetUnreadable.
	Reason string
	// OldVersion is the schema version found in the file; zero when the
	// file was unreadable or had no version.
	OldVersion uint64
}

// unusableError marks a file that must be reset rather than reported.
type unusableError struct {
	reason  string
	version uint64
	cause   error
}

func (e *unusableError) Error() string {
	return fmt.Sprintf("store: %s (version %d): %v",
		e.reason, e.version, e.cause)
}

// resetFile deletes the unusable file at path and opens a fresh store
// in its place. No backup is kept. The fresh open is not retried, so a
// bad disk fails instead of looping through resets.
func resetFile(path string, bad *unusableError) (*bolt.DB, error) {
	if err := removeFile(path); err != nil {
		return nil, fmt.Errorf("store: remove %s: %w", path, err)
	}
	klog.InfoS("state store reset", "component", "state",
		"operation", "reset", "reason", bad.reason,
		"oldVersion", bad.version, "supportedVersion", SchemaVersion,
		"cause", bad.cause.Error())
	db, err := openChecked(path)
	if err != nil {
		return nil, fmt.Errorf("store: open fresh %s: %w", path, err)
	}
	return db, nil
}

// openChecked opens path, verifies its schema and reads every page once.
// It creates a missing file and the buckets of a new one. It returns an
// *unusableError when the file should be reset.
func openChecked(path string) (*bolt.DB, error) {
	return openVerified(path, false)
}

// openForInspection opens the existing file at path read-only, for a
// replica that does not hold the Lease yet. It checks the pages and the
// schema version like openChecked but writes nothing: no file, no meta
// and no buckets. absent is true when there is no file yet.
func openForInspection(path string) (db *bolt.DB, absent bool, err error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, true, nil
	}
	db, err = openVerified(path, true)
	return db, false, err
}

// openWritable opens path for writing and creates any missing bucket.
// It is used after openForInspection already checked the pages.
func openWritable(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		if resettable(err) {
			err = &unusableError{reason: ResetUnreadable, cause: err}
		}
		return nil, err
	}
	if err := initialise(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func openVerified(path string, readOnly bool) (db *bolt.DB, err error) {
	db, err = bolt.Open(path, 0o600, &bolt.Options{
		Timeout: openTimeout, ReadOnly: readOnly,
	})
	if err != nil {
		if resettable(err) {
			err = &unusableError{reason: ResetUnreadable, cause: err}
		}
		return nil, err
	}
	// bbolt panics on some corrupt pages; that file is unreadable too.
	defer func() {
		if r := recover(); r != nil {
			_ = db.Close()
			db, err = nil, &unusableError{
				reason: ResetUnreadable, cause: fmt.Errorf("%v", r),
			}
		}
	}()
	// Pages are checked before initialise writes to the file.
	if err = checkPages(db); err != nil {
		_ = db.Close()
		return nil, &unusableError{reason: ResetUnreadable, cause: err}
	}
	if readOnly {
		err = db.View(checkVersion)
	} else {
		err = initialise(db)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// checkPages walks every page of db. bbolt reads pages lazily and
// panics on a damaged one, so without this check a corrupt page would
// crash a later read or write instead of resetting the file at Open.
// It reads the whole file once, which is fast next to the size cap.
func checkPages(db *bolt.DB) error {
	return db.View(func(tx *bolt.Tx) error {
		var first error
		// Drain every error: Check's goroutine holds the transaction
		// open until the channel is closed.
		for err := range tx.Check() {
			if first == nil {
				first = err
			}
		}
		return first
	})
}

// resettable reports whether an open error means the file content is bad,
// as opposed to the file being locked or inaccessible.
func resettable(err error) bool {
	var pathErr *fs.PathError
	var errno syscall.Errno
	switch {
	case errors.Is(err, berrors.ErrTimeout), errors.As(err, &pathErr),
		errors.As(err, &errno):
		return false
	default:
		return true
	}
}

// initialise creates every bucket in a new file and checks the version
// of an existing one.
func initialise(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		if err := checkVersion(tx); err != nil {
			return err
		}
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		// Buckets are created on every open: adding a data class is
		// not a layout change for existing classes.
		for _, spec := range specs {
			if _, err := tx.CreateBucketIfNotExists(
				[]byte(spec.name)); err != nil {
				return err
			}
		}
		return meta.Put(versionKey, writeUint(SchemaVersion))
	})
}

// checkVersion returns an *unusableError when the file has another
// schema version. A new, empty file has no version yet and passes.
func checkVersion(tx *bolt.Tx) error {
	version, known := storedVersion(tx)
	if known && version != SchemaVersion {
		return &unusableError{
			reason: ResetSchemaMismatch, version: version,
			cause: fmt.Errorf("supported version is %d", SchemaVersion),
		}
	}
	return nil
}

// storedVersion reads the file's version. known is false only for a new
// file; a file with data but no version counts as version 0.
func storedVersion(tx *bolt.Tx) (version uint64, known bool) {
	if meta := tx.Bucket(metaBucket); meta != nil {
		return readUint(meta.Get(versionKey)), true
	}
	hasData := false
	_ = tx.ForEach(func([]byte, *bolt.Bucket) error {
		hasData = true
		return nil
	})
	return 0, hasData
}

// removeFile deletes path and makes the removal durable.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// syncDir makes a rename durable.
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
