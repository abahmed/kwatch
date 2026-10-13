package unexplained

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/redact"
)

// The kinds of unexplained failure, as the metric label spells them.
const (
	KindEvent    = "event"
	KindIncident = "incident"
)

// Bounds.
const (
	// Window is the days counted.
	Window = 7
	// MaxRows bounds the (kind, mode, reason) rows; later ones share
	// the overflow row of their kind.
	MaxRows = 48
	// MaxSamples bounds the quoted samples kept per row.
	MaxSamples = 3
	// MaxSampleRunes cuts one sample.
	MaxSampleRunes = 160
	// MaxSeen bounds the incident IDs remembered, so an incident that
	// stays unclear is counted once and not at every update.
	MaxSeen = 256
	// Overflow names the mode and reason of the shared overflow row.
	Overflow  = "other"
	dayFormat = "2006-01-02"
)

// rowKey names one row.
type rowKey struct{ kind, mode, reason string }

// row is one reason's counts and samples.
type row struct {
	days    map[string]int
	samples []string
}

// Counts is the unexplained counts. It is safe for concurrent use: the
// informer handlers and the decision loop record, /status reads.
type Counts struct {
	mu   sync.Mutex
	rows map[rowKey]*row
	// seen lists the incidents already counted, oldest first.
	seen []string
	// high is the latest time of an event counted, so a restart that
	// lists the same events again does not count them twice.
	high time.Time
}

// New returns empty counts.
func New() *Counts { return &Counts{rows: map[rowKey]*row{}} }

// Record adds n occurrences of kind with the failure mode and quoted
// reason, seen at the time at, keeping sample as evidence.
func (c *Counts) Record(
	kind, mode, reason, sample string, n int, at time.Time,
) {
	if n <= 0 || (kind != KindEvent && kind != KindIncident) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordLocked(kind, mode, reason, sample, n, at)
}

func (c *Counts) recordLocked(
	kind, mode, reason, sample string, n int, at time.Time,
) {
	key := rowKey{kind, clean(mode, 64), clean(reason, 96)}
	r, known := c.rows[key]
	if !known {
		if len(c.rows) >= MaxRows {
			key = rowKey{kind, Overflow, Overflow}
			r = c.rows[key]
		}
		if r == nil {
			r = &row{days: map[string]int{}}
			c.rows[key] = r
		}
	}
	r.days[at.UTC().Format(dayFormat)] += n
	sample = clean(redact.Evidence(sample), MaxSampleRunes)
	if sample != "" && !contains(r.samples, sample) {
		r.samples = append(r.samples, sample)
		if over := len(r.samples) - MaxSamples; over > 0 {
			r.samples = r.samples[over:]
		}
	}
	if at.After(c.high) {
		c.high = at
	}
	c.pruneLocked(at)
}

// RecordIncident counts an incident once, however many updates it gets.
// It reports whether this call counted it.
func (c *Counts) RecordIncident(
	id, mode, reason, sample string, at time.Time,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if contains(c.seen, id) {
		return false
	}
	c.seen = append(c.seen, id)
	if over := len(c.seen) - MaxSeen; over > 0 {
		c.seen = c.seen[over:]
	}
	c.recordLocked(KindIncident, mode, reason, sample, 1, at)
	return true
}

// Latest is the time of the newest event counted. An event at or before
// it was counted already, or happened before counting began.
func (c *Counts) Latest() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.high
}

// clean puts text on one line and cuts it to limit characters.
func clean(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > limit {
		text = string(runes[:limit-1]) + "…"
	}
	return text
}

func contains(list []string, item string) bool {
	for _, have := range list {
		if have == item {
			return true
		}
	}
	return false
}

// windowStart is the first day, at midnight UTC, of the window ending at
// now.
func windowStart(now time.Time) time.Time {
	return now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -(Window - 1))
}

// pruneLocked drops the days outside the window ending at now, and the
// rows left without one.
func (c *Counts) pruneLocked(now time.Time) {
	oldest := windowStart(now).Format(dayFormat)
	for key, r := range c.rows {
		for day := range r.days {
			if day < oldest {
				delete(r.days, day)
			}
		}
		if len(r.days) == 0 {
			delete(c.rows, key)
		}
	}
}

// Row is one reason's total over the window.
type Row struct {
	Kind    string   `json:"kind"`
	Mode    string   `json:"mode"`
	Reason  string   `json:"reason"`
	Count   int      `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

// Summary is the window ending at now.
type Summary struct {
	Events    int `json:"events"`
	Incidents int `json:"incidents"`
	// Top are the largest rows, most first.
	Top []Row `json:"top,omitempty"`
}

// Summary totals the window ending at now and lists the top rows.
func (c *Counts) Summary(now time.Time, top int) Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	oldest := windowStart(now).Format(dayFormat)
	var s Summary
	var rows []Row
	for key, r := range c.rows {
		out := Row{Kind: key.kind, Mode: key.mode, Reason: key.reason,
			Samples: append([]string(nil), r.samples...)}
		for day, n := range r.days {
			if day >= oldest {
				out.Count += n
			}
		}
		if out.Count == 0 {
			continue
		}
		if key.kind == KindEvent {
			s.Events += out.Count
		} else {
			s.Incidents += out.Count
		}
		rows = append(rows, out)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Kind+a.Mode+a.Reason < b.Kind+b.Mode+b.Reason
	})
	if len(rows) > top {
		rows = rows[:top]
	}
	s.Top = rows
	return s
}

// wire is the stored form.
type wire struct {
	Rows []wireRow `json:"rows"`
	Seen []string  `json:"seen,omitempty"`
	High time.Time `json:"high,omitempty"`
}

type wireRow struct {
	Kind    string         `json:"kind"`
	Mode    string         `json:"mode"`
	Reason  string         `json:"reason"`
	Days    map[string]int `json:"days"`
	Samples []string       `json:"samples,omitempty"`
}

// MarshalJSON writes the rows, the incidents counted and the newest
// event time.
func (c *Counts) MarshalJSON() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := wire{Seen: c.seen, High: c.high}
	for key, r := range c.rows {
		out.Rows = append(out.Rows, wireRow{key.kind, key.mode, key.reason,
			r.days, r.samples})
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		return a.Kind+a.Mode+a.Reason < b.Kind+b.Mode+b.Reason
	})
	return json.Marshal(out)
}

// UnmarshalJSON reads what MarshalJSON wrote. Rows past the bound, days
// that do not parse and samples past the bound are skipped, so a damaged
// value cannot grow the counts.
func (c *Counts) UnmarshalJSON(data []byte) error {
	var in wire
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = map[rowKey]*row{}
	c.high = in.High
	c.seen = in.Seen
	if len(c.seen) > MaxSeen {
		c.seen = c.seen[len(c.seen)-MaxSeen:]
	}
	for _, w := range in.Rows {
		if len(c.rows) >= MaxRows ||
			(w.Kind != KindEvent && w.Kind != KindIncident) {
			continue
		}
		r := &row{days: map[string]int{}}
		for day, n := range w.Days {
			if _, err := time.Parse(dayFormat, day); err == nil && n > 0 {
				r.days[day] = n
			}
		}
		if len(r.days) == 0 {
			continue
		}
		if len(w.Samples) > MaxSamples {
			w.Samples = w.Samples[len(w.Samples)-MaxSamples:]
		}
		for _, sample := range w.Samples {
			r.samples = append(r.samples, clean(sample, MaxSampleRunes))
		}
		c.rows[rowKey{w.Kind, w.Mode, w.Reason}] = r
	}
	return nil
}
