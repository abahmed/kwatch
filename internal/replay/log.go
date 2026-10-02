package replay

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Format and Version identify an observation log. The header line carries
// both; readers reject any other format and any version they do not know.
const (
	Format  = "kwatch-observations"
	Version = 1
)

// maxLineBytes bounds one log line. Observations carry bounded values, so
// a longer line means the file is not an observation log. The read
// buffer starts at initialLineBytes and grows up to it.
const (
	maxLineBytes     = 1 << 20
	initialLineBytes = 64 << 10
)

// Header is the first line of an observation log.
type Header struct {
	Format  string    `json:"format"`
	Version int       `json:"version"`
	Start   time.Time `json:"start"`
}

// Entry is one recorded observation and the time the pipeline received it.
type Entry struct {
	At          time.Time             `json:"at"`
	Observation inventory.Observation `json:"obs"`
}

// Log is a decoded observation log. Entries are in non-decreasing At
// order, none before Start.
type Log struct {
	Start   time.Time
	Entries []Entry
}

// Write encodes a complete log.
func Write(w io.Writer, log Log) error {
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(newHeader(log.Start)); err != nil {
		return err
	}
	for _, entry := range log.Entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func newHeader(start time.Time) Header {
	return Header{Format: Format, Version: Version, Start: start.UTC()}
}

// Read decodes a log and validates its header and ordering.
func Read(r io.Reader) (Log, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, initialLineBytes), maxLineBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return Log{}, fmt.Errorf("replay: read header: %w", err)
		}
		return Log{}, errors.New("replay: empty log")
	}
	header, err := parseHeader(scanner.Bytes())
	if err != nil {
		return Log{}, err
	}
	log := Log{Start: header.Start}
	previous := header.Start
	for line := 2; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		entry, err := parseEntry(scanner.Bytes(), previous)
		if err != nil {
			return Log{}, fmt.Errorf("replay: line %d: %w", line, err)
		}
		previous = entry.At
		log.Entries = append(log.Entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return Log{}, fmt.Errorf("replay: read: %w", err)
	}
	return log, nil
}

func parseHeader(line []byte) (Header, error) {
	var header Header
	if err := json.Unmarshal(line, &header); err != nil {
		return Header{}, fmt.Errorf("replay: header: %w", err)
	}
	if header.Format != Format {
		return Header{}, fmt.Errorf("replay: format %q is not %q",
			header.Format, Format)
	}
	if header.Version != Version {
		return Header{}, fmt.Errorf("replay: unsupported version %d",
			header.Version)
	}
	if header.Start.IsZero() {
		return Header{}, errors.New("replay: header without start")
	}
	return header, nil
}

func parseEntry(line []byte, previous time.Time) (Entry, error) {
	var entry Entry
	if err := json.Unmarshal(line, &entry); err != nil {
		return Entry{}, err
	}
	if entry.Observation.Kind == 0 {
		return Entry{}, errors.New("entry without observation")
	}
	if entry.At.Before(previous) {
		return Entry{}, fmt.Errorf("entry at %s is before %s",
			entry.At.Format(time.RFC3339Nano),
			previous.Format(time.RFC3339Nano))
	}
	return entry, nil
}
