package storage

import (
	"bytes"
	"time"

	bolt "go.etcd.io/bbolt"
)

// PutAll stores every item in one transaction and keeps keys that items
// does not name. To make items the whole bucket, use a Mirror.
func (k Keyed[T]) PutAll(items map[string]Item[T]) error {
	encoded := make(map[string][]byte, len(items))
	for key, item := range items {
		if key == "" {
			return ErrEmptyKey
		}
		data, err := encode(item.Value, k.store.now(), item.Expires)
		if err != nil {
			return err
		}
		encoded[key] = data
	}
	if len(encoded) == 0 {
		return nil
	}
	return k.store.update(func(tx *bolt.Tx) error {
		k.store.touch(k.bucket)
		bucket := tx.Bucket([]byte(k.bucket))
		for key, data := range encoded {
			if err := bucket.Put([]byte(key), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// Entry is one log entry for AppendAll.
type Entry[T any] struct {
	Entity string
	At     time.Time
	Value  T
}

// AppendAll adds every entry in one transaction, in order. Entries with
// the same entity and time keep their order. Nothing is written when one
// entry is invalid.
func (l Log[T]) AppendAll(entries []Entry[T]) error {
	encoded, err := l.encodeEntries(entries)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	return l.store.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(l.bucket))
		for i, entry := range entries {
			seq, err := bucket.NextSequence()
			if err != nil {
				return err
			}
			key := logKey(entry.Entity, entry.At, seq)
			if err := bucket.Put(key, encoded[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// encodeEntries validates and encodes entries before any write.
func (l Log[T]) encodeEntries(entries []Entry[T]) ([][]byte, error) {
	encoded := make([][]byte, len(entries))
	for i, entry := range entries {
		if entry.Entity == "" {
			return nil, ErrEmptyKey
		}
		if bytes.IndexByte([]byte(entry.Entity), 0) >= 0 {
			return nil, ErrBadEntity
		}
		data, err := encode(entry.Value, l.store.now(), time.Time{})
		if err != nil {
			return nil, err
		}
		encoded[i] = data
	}
	return encoded, nil
}
