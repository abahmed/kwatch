package scorecard

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/audit"
)

// maxLineBytes bounds one audit line; entries carry timelines and can be
// several kilobytes.
const maxLineBytes = 4 << 20

// Parse reads audit entries from JSON lines or from a log export CSV whose
// last column holds the log line. Lines that are not audit entries, such as
// ordinary klog output, are skipped. Entries are returned in time order.
func Parse(r io.Reader) ([]audit.Entry, error) {
	reader := bufio.NewReaderSize(r, 64<<10)
	head, err := reader.Peek(1)
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []audit.Entry
	if head[0] == '"' || isCSVHeader(reader) {
		entries, err = parseCSV(reader)
	} else {
		entries, err = parseJSONLines(reader)
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return entries, nil
}

func isCSVHeader(reader *bufio.Reader) bool {
	line, err := reader.Peek(16)
	if err != nil && len(line) == 0 {
		return false
	}
	return strings.HasPrefix(string(line), "Date,")
}

func parseCSV(r io.Reader) ([]audit.Entry, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	var entries []audit.Entry
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
		if len(record) == 0 {
			continue
		}
		if entry, ok := decodeEntry(record[len(record)-1]); ok {
			entries = append(entries, entry)
		}
	}
}

func parseJSONLines(r io.Reader) ([]audit.Entry, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	var entries []audit.Entry
	for scanner.Scan() {
		if entry, ok := decodeEntry(scanner.Text()); ok {
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}

func decodeEntry(line string) (audit.Entry, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") {
		return audit.Entry{}, false
	}
	var entry audit.Entry
	if json.Unmarshal([]byte(line), &entry) != nil ||
		entry.Action == "" || entry.Incident == "" {
		return audit.Entry{}, false
	}
	return entry, true
}
