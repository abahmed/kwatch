package storage

import (
	"bytes"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ErrEmptyKey is returned for an empty key or entity.
var ErrEmptyKey = errors.New("store: empty key")

// Keyed holds the latest value of type T per key in one bucket. Get it
// from a constructor such as IncidentRecords. Values are JSON encoded.
//
// A value that no longer decodes is skipped (reported as absent) and
// counted in Stats; a value past its expiry is also reported as absent
// until the compactor deletes it.
type Keyed[T any] struct {
	store  *Store
	bucket Bucket
}

// Item is a value with an optional expiry, for PutAll and Mirror.
type Item[T any] struct {
	// Value is what is stored.
	Value T
	// Expires is when the value may be deleted; zero keeps it.
	Expires time.Time
}

// Put stores value under key with no expiry, replacing any old value.
func (k Keyed[T]) Put(key string, value T) error {
	return k.PutUntil(key, value, time.Time{})
}

// PutUntil stores value under key; the compactor deletes it after
// expires. A zero expires keeps it forever.
func (k Keyed[T]) PutUntil(key string, value T, expires time.Time) error {
	if key == "" {
		return ErrEmptyKey
	}
	data, err := encode(value, k.store.now(), expires)
	if err != nil {
		return err
	}
	return k.store.update(func(tx *bolt.Tx) error {
		k.store.touch(k.bucket)
		return tx.Bucket([]byte(k.bucket)).Put([]byte(key), data)
	})
}

// Get returns the value under key and whether it was found.
func (k Keyed[T]) Get(key string) (T, bool, error) {
	var out T
	found := false
	err := k.store.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(k.bucket))
		if bucket == nil {
			return nil // a read-only file from before this bucket
		}
		data := bucket.Get([]byte(key))
		if data == nil {
			return nil
		}
		var ok bool
		out, ok = k.decode(data)
		found = ok
		return nil
	})
	return out, found, err
}

// Delete removes key. Deleting a missing key is not an error.
func (k Keyed[T]) Delete(key string) error {
	return k.DeleteMany([]string{key})
}

// DeleteMany removes every key in one transaction. Missing keys are not
// an error, and an empty list writes nothing.
func (k Keyed[T]) DeleteMany(keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return k.store.update(func(tx *bolt.Tx) error {
		k.store.touch(k.bucket)
		bucket := tx.Bucket([]byte(k.bucket))
		for _, key := range keys {
			if err := bucket.Delete([]byte(key)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Range visits every value whose key starts with prefix, in key order.
// An empty prefix visits the whole bucket. Returning an error from visit
// stops the walk and returns that error.
func (k Keyed[T]) Range(
	prefix string, visit func(key string, value T) error,
) error {
	return k.store.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(k.bucket))
		if bucket == nil {
			return nil // a read-only file from before this bucket
		}
		cursor := bucket.Cursor()
		p := []byte(prefix)
		for key, data := cursor.Seek(p); key != nil &&
			bytes.HasPrefix(key, p); key, data = cursor.Next() {
			value, ok := k.decode(data)
			if !ok {
				continue
			}
			if err := visit(string(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

// decode returns the value, or false for a corrupt or expired one.
func (k Keyed[T]) decode(data []byte) (T, bool) {
	var out T
	h, err := decodeValue(data, &out)
	if err != nil {
		k.store.skipCorrupt(k.bucket, err)
		return out, false
	}
	return out, !h.expired(k.store.now())
}
