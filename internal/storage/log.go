package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ErrBadEntity is returned for an entity containing a NUL byte, which
// ends the entity inside a log key.
var ErrBadEntity = errors.New("store: entity contains a NUL byte")

// Log holds time-ordered entries of type T per entity in one bucket. Get
// it from a constructor such as ChangeLog.
//
// Keys are entity, NUL, entry time, sequence (both big-endian), so all
// entries of one entity sit together, oldest first, and a time range is
// one cursor seek. The compactor deletes entries older than the bucket's
// retention, measured from the entry time. Corrupt entries are skipped
// and counted in Stats.
type Log[T any] struct {
	store  *Store
	bucket Bucket
}

// logSuffix is the time and sequence after the entity and separator.
const logSuffix = 16

// Append adds value for entity at time at. Entries with the same entity
// and time keep their append order. Times before 1970 are not supported.
func (l Log[T]) Append(entity string, at time.Time, value T) error {
	if entity == "" {
		return ErrEmptyKey
	}
	if bytes.IndexByte([]byte(entity), 0) >= 0 {
		return ErrBadEntity
	}
	data, err := encode(value, l.store.now(), time.Time{})
	if err != nil {
		return err
	}
	return l.store.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(l.bucket))
		seq, err := bucket.NextSequence()
		if err != nil {
			return err
		}
		return bucket.Put(logKey(entity, at, seq), data)
	})
}

// Range visits entity's entries with since <= at < until, oldest first.
// A zero since or until leaves that side open. Returning an error from
// visit stops the walk and returns that error.
func (l Log[T]) Range(
	entity string, since, until time.Time,
	visit func(at time.Time, value T) error,
) error {
	prefix := entityPrefix(entity)
	start := prefix
	if !since.IsZero() {
		start = logKey(entity, since, 0)
	}
	return l.store.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(l.bucket))
		if bucket == nil {
			return nil // a read-only file from before this bucket
		}
		return l.walk(bucket.Cursor(), start, prefix, until, visit)
	})
}

// walk visits the entries from start that share prefix and are older
// than until (open when zero), skipping corrupt values.
func (l Log[T]) walk(
	cursor *bolt.Cursor, start, prefix []byte, until time.Time,
	visit func(at time.Time, value T) error,
) error {
	for k, data := cursor.Seek(start); k != nil &&
		bytes.HasPrefix(k, prefix); k, data = cursor.Next() {
		at, _ := logTime(k)
		if !until.IsZero() && !at.Before(until) {
			return nil
		}
		var value T
		if _, err := decodeValue(data, &value); err != nil {
			l.store.skipCorrupt(l.bucket, err)
			continue
		}
		if err := visit(at, value); err != nil {
			return err
		}
	}
	return nil
}

func entityPrefix(entity string) []byte {
	return append([]byte(entity), 0)
}

func logKey(entity string, at time.Time, seq uint64) []byte {
	key := entityPrefix(entity)
	key = binary.BigEndian.AppendUint64(key, uint64(at.UnixNano()))
	return binary.BigEndian.AppendUint64(key, seq)
}

// logTime reads the entry time from the end of a log key.
func logTime(key []byte) (time.Time, bool) {
	if len(key) < logSuffix+1 {
		return time.Time{}, false
	}
	start := len(key) - logSuffix
	nanos := binary.BigEndian.Uint64(key[start : start+8])
	return time.Unix(0, int64(nanos)), true
}
