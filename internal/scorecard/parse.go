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
	entries, _, err := ParseCounted(r)
	return entries, err
}

// ParseCounted is Parse that also returns how many non-blank lines (or CSV
// records) were skipped because they were not audit entries.
func ParseCounted(r io.Reader) ([]audit.Entry, int, error) {
	reader := bufio.NewReaderSize(r, 64<<10)
	head, err := reader.Peek(1)
	if errors.Is(err, io.EOF) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var entries []audit.Entry
	var skipped int
	if head[0] == '"' || isCSVHeader(reader) {
		entries, skipped, err = parseCSV(reader)
	} else {
		entries, skipped, err = parseJSONLines(reader)
	}
	if err != nil {
		return nil, 0, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return entries, skipped, nil
}

func isCSVHeader(reader *bufio.Reader) bool {
	line, err := reader.Peek(16)
	if err != nil && len(line) == 0 {
		return false
	}
	return strings.HasPrefix(string(line), "Date,")
}

func parseCSV(r io.Reader) ([]audit.Entry, int, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	var entries []audit.Entry
	skipped := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return entries, skipped, nil
		}
		if err != nil {
			return nil, 0, err
		}
		if len(record) == 0 {
			continue
		}
		if entry, ok := decodeEntry(record[len(record)-1]); ok {
			entries = append(entries, entry)
		} else {
			skipped++
		}
	}
}

func parseJSONLines(r io.Reader) ([]audit.Entry, int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	var entries []audit.Entry
	skipped := 0
	for scanner.Scan() {
		line := scanner.Text()
		if entry, ok := decodeEntry(line); ok {
			entries = append(entries, entry)
		} else if strings.TrimSpace(line) != "" {
			skipped++
		}
	}
	return entries, skipped, scanner.Err()
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
