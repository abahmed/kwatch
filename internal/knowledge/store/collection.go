package store

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Collection names one bucket. Keyed collections hold the latest value per
// key; history collections hold time-ordered entries per partition.
type Collection string

// Collections used by the core.
const (
	Decisions Collection = "decisions"
	Problems  Collection = "problems"
	Changes   Collection = "changes"
	Baselines Collection = "baselines"
	Evidence  Collection = "evidence"
	Series    Collection = "series"
	Snapshot  Collection = "snapshot"
)

var collections = []Collection{
	Decisions, Problems, Changes, Baselines, Evidence, Series, Snapshot,
}

// partitionSeparator ends a partition inside history keys. Partitions are
// entity keys, which never contain a NUL byte.
const partitionSeparator = 0x00

// ErrUnknownCollection is returned for a collection the schema lacks.
var ErrUnknownCollection = errors.New("unknown collection")

// Put stores value under key, replacing any previous value.
func (s *Store) Put(c Collection, key string, value any) error {
	data, err := s.encode(value)
	if err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), data)
	})
}

// Get loads the value under key into out. It reports false when absent.
func (s *Store) Get(c Collection, key string, out any) (bool, error) {
	found := false
	err := s.view(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		data := bucket.Get([]byte(key))
		if data == nil {
			return nil
		}
		found = true
		return decode(data, out)
	})
	return found, err
}

// Delete removes key.
func (s *Store) Delete(c Collection, key string) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		return bucket.Delete([]byte(key))
	})
}

// ForEach visits keyed values whose key starts with prefix, in key order.
func (s *Store) ForEach(
	c Collection, prefix string,
	visit func(key string, decode func(out any) error) error,
) error {
	return s.view(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		cursor := bucket.Cursor()
		p := []byte(prefix)
		for k, v := cursor.Seek(p); k != nil && bytes.HasPrefix(k, p); k, v =
			cursor.Next() {
			data := v
			if err := visit(string(k), func(out any) error {
				return decode(data, out)
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Append adds a time-ordered entry to partition.
func (s *Store) Append(
	c Collection, partition string, at time.Time, value any,
) error {
	data, err := s.encode(value)
	if err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		seq, err := bucket.NextSequence()
		if err != nil {
			return err
		}
		return bucket.Put(historyKey(partition, at, seq), data)
	})
}

// Range visits partition entries with since <= at < until, oldest first.
// A zero since or until leaves that side unbounded. Times before 1970 are
// not supported.
func (s *Store) Range(
	c Collection, partition string, since, until time.Time,
	visit func(at time.Time, decode func(out any) error) error,
) error {
	return s.view(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		prefix := partitionPrefix(partition)
		start := prefix
		if !since.IsZero() {
			start = historyKey(partition, since, 0)
		}
		cursor := bucket.Cursor()
		for k, v := cursor.Seek(start); k != nil &&
			bytes.HasPrefix(k, prefix); k, v = cursor.Next() {
			at := historyTime(k, len(prefix))
			if !until.IsZero() && !at.Before(until) {
				return nil
			}
			data := v
			if err := visit(at, func(out any) error {
				return decode(data, out)
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// envelope prefixes every value with its write time, used by retention.
func (s *Store) encode(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("store: encode: %w", err)
	}
	out := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint64(out, uint64(s.now().UnixNano()))
	return append(out, payload...), nil
}

func decode(data []byte, out any) error {
	if len(data) < 8 {
		return errors.New("store: corrupt value")
	}
	return json.Unmarshal(data[8:], out)
}

func writtenAt(data []byte) time.Time {
	if len(data) < 8 {
		return time.Time{}
	}
	return time.Unix(0, int64(binary.BigEndian.Uint64(data[:8])))
}

func bucketFor(tx *bolt.Tx, c Collection) (*bolt.Bucket, error) {
	bucket := tx.Bucket([]byte(c))
	if bucket == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCollection, c)
	}
	return bucket, nil
}

func partitionPrefix(partition string) []byte {
	return append([]byte(partition), partitionSeparator)
}

func historyKey(partition string, at time.Time, seq uint64) []byte {
	key := partitionPrefix(partition)
	key = binary.BigEndian.AppendUint64(key, uint64(at.UnixNano()))
	return binary.BigEndian.AppendUint64(key, seq)
}

func historyTime(key []byte, prefixLength int) time.Time {
	if len(key) < prefixLength+8 {
		return time.Time{}
	}
	nanos := binary.BigEndian.Uint64(key[prefixLength : prefixLength+8])
	return time.Unix(0, int64(nanos))
}

// ReplaceAll makes values the complete content of a keyed collection in
// one transaction: keys absent from values are deleted.
func (s *Store) ReplaceAll(c Collection, values map[string]any) error {
	encoded := make(map[string][]byte, len(values))
	for key, value := range values {
		data, err := s.encode(value)
		if err != nil {
			return err
		}
		encoded[key] = data
	}
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := bucketFor(tx, c)
		if err != nil {
			return err
		}
		var stale [][]byte
		cursor := bucket.Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			if _, keep := encoded[string(k)]; !keep {
				stale = append(stale, append([]byte(nil), k...))
			}
		}
		for _, key := range stale {
			if err := bucket.Delete(key); err != nil {
				return err
			}
		}
		for key, data := range encoded {
			if err := bucket.Put([]byte(key), data); err != nil {
				return err
			}
		}
		return nil
	})
}
