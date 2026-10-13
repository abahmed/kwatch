package flight

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/storage"
)

// Stream is the log entity every batch is stored under: one ring.
const Stream = "obs"

// Bounds of one stored batch. A batch holds at most maxBatchEntries
// entries recorded within one flush interval, so its key time (the time
// of its first entry) is within maxBatchSpan before any entry inside.
// maxBatchBytes bounds what a reader inflates from one value.
const (
	maxBatchEntries = 2048
	maxBatchSpan    = time.Minute
	maxBatchBytes   = 32 << 20
)

// Batch is one stored value: gzip-compressed JSON lines, one replay.Entry
// per line.
type Batch struct {
	Z []byte `json:"z"`
	N int    `json:"n"`
}

// Ring is the flight bucket of a state file.
type Ring struct {
	log storage.Log[Batch]
}

// NewRing returns the ring in s.
func NewRing(s *storage.Store) Ring {
	return Ring{log: storage.FlightLog[Batch](s)}
}

// Append stores entries, which must be in time order, as one batch.
func (r Ring) Append(entries []replay.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	var raw bytes.Buffer
	zip := gzip.NewWriter(&raw)
	encoder := json.NewEncoder(zip)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}
	if err := zip.Close(); err != nil {
		return err
	}
	return r.log.Append(Stream, entries[0].At,
		Batch{Z: raw.Bytes(), N: len(entries)})
}

// errStop ends a walk early without an error.
var errStop = errors.New("flight: stop")

// Walk visits the entries with since <= At < until, oldest first. A zero
// bound is open. A batch that does not decode is skipped.
func (r Ring) Walk(
	since, until time.Time, visit func(replay.Entry) error,
) error {
	from := since
	if !from.IsZero() {
		from = from.Add(-maxBatchSpan)
	}
	err := r.log.Range(Stream, from, until,
		func(_ time.Time, batch Batch) error {
			entries, err := decodeBatch(batch)
			if err != nil {
				return nil
			}
			for _, entry := range entries {
				if !since.IsZero() && entry.At.Before(since) {
					continue
				}
				if !until.IsZero() && !entry.At.Before(until) {
					return nil
				}
				if err := visit(entry); err != nil {
					return err
				}
			}
			return nil
		})
	if errors.Is(err, errStop) {
		return nil
	}
	return err
}

// decodeBatch inflates one batch.
func decodeBatch(batch Batch) ([]replay.Entry, error) {
	zip, err := gzip.NewReader(bytes.NewReader(batch.Z))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zip.Close() }()
	decoder := json.NewDecoder(io.LimitReader(zip, maxBatchBytes))
	var out []replay.Entry
	for decoder.More() {
		var entry replay.Entry
		if err := decoder.Decode(&entry); err != nil {
			return nil, fmt.Errorf("flight: batch: %w", err)
		}
		out = append(out, entry)
	}
	return out, nil
}
