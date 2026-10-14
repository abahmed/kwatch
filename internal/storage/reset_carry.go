package storage

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"
	"k8s.io/klog/v2"
)

// carriedBuckets are the classes copied into the fresh file after a schema
// change. They hold the work an upgrade must not lose: queued pages and
// resolves (outbox) and the chat and ticket thread ids (threads). Their
// values carry their own version field, and the code that reads them
// skips or drops a value in a layout it does not know. Every other class
// is rebuilt from the cluster, so it starts empty.
var carriedBuckets = []Bucket{Outbox, Threads}

// carryRecords copies the carried buckets of the old file at aside into
// db and returns how many records it copied. It is best effort: a file
// that cannot be read, or a record that is not valid, is skipped and
// logged, because a failed copy must never stop the fresh start.
func carryRecords(aside string, db *bolt.DB) int {
	old, err := bolt.Open(aside, 0o600, &bolt.Options{
		Timeout: openTimeout, ReadOnly: true,
	})
	if err != nil {
		klog.ErrorS(err, "state store reset: cannot read the old file",
			"component", "state", "operation", "carry")
		return 0
	}
	defer func() { _ = old.Close() }()
	carried := 0
	err = old.View(func(from *bolt.Tx) error {
		return db.Update(func(to *bolt.Tx) error {
			for _, name := range carriedBuckets {
				n, err := copyBucket(from, to, name)
				if err != nil {
					return err
				}
				carried += n
			}
			return nil
		})
	})
	if err != nil {
		klog.ErrorS(err, "state store reset: copy failed",
			"component", "state", "operation", "carry")
		return 0
	}
	return carried
}

// copyBucket copies the valid values of one bucket and returns the count.
func copyBucket(from, to *bolt.Tx, name Bucket) (int, error) {
	source := from.Bucket([]byte(name))
	target := to.Bucket([]byte(name))
	if source == nil || target == nil {
		return 0, nil
	}
	copied := 0
	err := source.ForEach(func(key, value []byte) error {
		if _, ok := readHeader(value); !ok ||
			!json.Valid(value[headerSize:]) {
			return nil
		}
		copied++
		return target.Put(append([]byte(nil), key...),
			append([]byte(nil), value...))
	})
	return copied, err
}
