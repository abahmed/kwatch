package storage

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"
)

// headerSize is the fixed prefix of every stored value: the write time
// and the expiry time, both Unix nanoseconds (zero expiry means never).
const headerSize = 16

// errCorrupt marks a value that cannot be decoded.
var errCorrupt = errors.New("store: corrupt value")

// header is the decoded value prefix.
type header struct {
	written time.Time
	expires time.Time
}

// expired reports whether the value has an expiry at or before now.
func (h header) expired(now time.Time) bool {
	return !h.expires.IsZero() && !h.expires.After(now)
}

func encode(value any, written, expires time.Time) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("store: encode: %w", err)
	}
	out := make([]byte, headerSize, headerSize+len(payload))
	binary.BigEndian.PutUint64(out, uint64(written.UnixNano()))
	binary.BigEndian.PutUint64(out[8:], unixNanos(expires))
	return append(out, payload...), nil
}

func unixNanos(at time.Time) uint64 {
	if at.IsZero() {
		return 0
	}
	return uint64(at.UnixNano())
}

func readHeader(data []byte) (header, bool) {
	if len(data) < headerSize {
		return header{}, false
	}
	h := header{written: time.Unix(0,
		int64(binary.BigEndian.Uint64(data[:8])))}
	if nanos := binary.BigEndian.Uint64(data[8:16]); nanos != 0 {
		h.expires = time.Unix(0, int64(nanos))
	}
	return h, true
}

// decodeValue unmarshals data into out. Any failure is errCorrupt.
func decodeValue(data []byte, out any) (header, error) {
	h, ok := readHeader(data)
	if !ok {
		return header{}, errCorrupt
	}
	if err := json.Unmarshal(data[headerSize:], out); err != nil {
		return header{}, fmt.Errorf("%w: %w", errCorrupt, err)
	}
	return h, nil
}

// counters are the store's lifetime totals for this process.
type counters struct {
	corrupt atomic.Uint64
	expired atomic.Uint64
	evicted atomic.Uint64
	logged  sync.Map // Bucket -> struct{}: corrupt value already logged
	// writeTxs and scans count write transactions and compactor scans;
	// tests use them to bound the cost of a pass.
	writeTxs atomic.Uint64
	scans    atomic.Uint64
	overCap  atomic.Bool
	// rewrites counts online rewrites that shrank the file; fileBytes
	// and freeBytes are the size of the file and of its free pages after
	// the last compactor pass.
	rewrites  atomic.Uint64
	fileBytes atomic.Int64
	freeBytes atomic.Int64
	// rewriteDue is set while the file waits for a startup rewrite.
	rewriteDue atomic.Bool
}

// skipCorrupt counts one undecodable value and logs it once per bucket
// per process, so a damaged bucket cannot flood the log.
func (s *Store) skipCorrupt(b Bucket, err error) {
	s.counters.corrupt.Add(1)
	if _, seen := s.counters.logged.LoadOrStore(b, struct{}{}); seen {
		return
	}
	klog.ErrorS(err, "skipping corrupt stored value",
		"component", "state", "operation", "decode",
		"bucket", string(b))
}

// Stats are the store's counters since Open.
type Stats struct {
	// CorruptRecords counts values skipped because they did not decode.
	CorruptRecords uint64
	// Expired counts values the compactor removed by retention.
	Expired uint64
	// Evicted counts values the compactor removed to meet a size cap.
	Evicted uint64
	// OverCap is true when the last compactor pass left the store over
	// its size cap because pinned data (open incidents, baselines,
	// fingerprints, state, threads) needs the room. It is a degraded
	// state to report, not something more eviction can fix.
	OverCap bool
	// RewriteDue is true when the last compactor pass found the file so
	// far over its size cap that the next start rewrites it.
	RewriteDue bool
	// Rewrites counts the times the file was rewritten while running to
	// give back free pages.
	Rewrites uint64
	// FileBytes is the size of the state file after the last pass, and
	// FreeBytes the part of it that is free pages inside the file.
	FileBytes, FreeBytes int64
}

// Stats returns the counters since Open.
func (s *Store) Stats() Stats {
	return Stats{
		CorruptRecords: s.counters.corrupt.Load(),
		Expired:        s.counters.expired.Load(),
		Evicted:        s.counters.evicted.Load(),
		OverCap:        s.counters.overCap.Load(),
		RewriteDue:     s.counters.rewriteDue.Load(),
		Rewrites:       s.counters.rewrites.Load(),
		FileBytes:      s.counters.fileBytes.Load(),
		FreeBytes:      s.counters.freeBytes.Load(),
	}
}
