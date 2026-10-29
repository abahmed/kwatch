package storage

import (
	"encoding/json"
	"fmt"

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
// db and returns how many records it copied and how many it skipped. It
// is best effort and never fails the reset: a file that cannot be opened,
// a bucket that cannot be read (bbolt panics on a damaged page, which is
// recovered here) and a record that is not valid are skipped and logged.
// Each bucket is copied in its own transaction, so one damaged bucket
// does not lose the others.
func carryRecords(aside string, db *bolt.DB) (carried, skipped int) {
	old, err := bolt.Open(aside, 0o600, &bolt.Options{
		Timeout: openTimeout, ReadOnly: true,
	})
	if err != nil {
		klog.ErrorS(err, "state store reset: cannot read the old file",
			"component", "state", "operation", "carry")
		return 0, 0
	}
	defer func() { _ = old.Close() }()
	for _, name := range carriedBuckets {
		n, bad := carryBucket(old, db, name)
		carried += n
		skipped += bad
	}
	return carried, skipped
}

// carryBucket copies one bucket in its own transactions. A panic or error
// leaves the bucket partly or wholly uncopied and is logged, not returned.
func carryBucket(old, db *bolt.DB, name Bucket) (copied, skipped int) {
	defer func() {
		if r := recover(); r != nil {
			klog.ErrorS(fmt.Errorf("%v", r),
				"state store reset: bucket unreadable",
				"component", "state", "operation", "carry",
				"bucket", string(name))
			copied, skipped = 0, 0
		}
	}()
	err := old.View(func(from *bolt.Tx) error {
		return db.Update(func(to *bolt.Tx) error {
			var err error
			copied, skipped, err = copyBucket(from, to, name)
			return err
		})
	})
	if err != nil {
		klog.ErrorS(err, "state store reset: copy failed",
			"component", "state", "operation", "carry",
			"bucket", string(name))
		return 0, 0
	}
	return copied, skipped
}

// copyBucket copies the valid values of one bucket and returns the
// copied and skipped counts.
func copyBucket(from, to *bolt.Tx, name Bucket) (int, int, error) {
	source := from.Bucket([]byte(name))
	target := to.Bucket([]byte(name))
	if source == nil || target == nil {
		return 0, 0, nil
	}
	copied, skipped := 0, 0
	err := source.ForEach(func(key, value []byte) error {
		if _, ok := readHeader(value); !ok ||
			!json.Valid(value[headerSize:]) {
			skipped++
			return nil
		}
		copied++
		return target.Put(append([]byte(nil), key...),
			append([]byte(nil), value...))
	})
	return copied, skipped, err
}
