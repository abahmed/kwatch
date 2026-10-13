package selfcheck

import (
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Bounds. A rule has at most Window days of three numbers, and at most
// MaxRules rules are tracked by name; the rest share OverflowRule. The
// whole value stays under a few kilobytes.
const (
	Window       = 7
	MaxRules     = 48
	OverflowRule = "other rules"
	dayFormat    = "2006-01-02"
)

// tally is one day's counts: confirmed, other, none.
type tally [3]int

// Counts holds the outcomes of resolved incidents per rule and day. It is
// safe for concurrent use: the decision loop records, /status reads.
type Counts struct {
	mu    sync.Mutex
	rules map[string]map[string]tally
}

// NewCounts returns empty counts.
func NewCounts() *Counts {
	return &Counts{rules: map[string]map[string]tally{}}
}

// Record adds one resolved incident of rule that ended with outcome on
// the day of at. Days older than the window are forgotten.
func (c *Counts) Record(rule string, at time.Time, outcome Outcome) {
	slot, ok := slotOf(outcome)
	if !ok || rule == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, known := c.rules[rule]; !known && len(c.rules) >= MaxRules {
		rule = OverflowRule
	}
	days := c.rules[rule]
	if days == nil {
		days = map[string]tally{}
		c.rules[rule] = days
	}
	key := at.UTC().Format(dayFormat)
	t := days[key]
	t[slot]++
	days[key] = t
	c.pruneLocked(at)
}

func slotOf(o Outcome) (int, bool) {
	switch o {
	case CauseConfirmed:
		return 0, true
	case OtherChanged:
		return 1, true
	case NothingChanged:
		return 2, true
	}
	return 0, false
}

// pruneLocked drops the days that fall outside the window ending at now,
// and the rules left without a day.
func (c *Counts) pruneLocked(now time.Time) {
	oldest := windowStart(now).Format(dayFormat)
	for rule, days := range c.rules {
		for key := range days {
			if key < oldest {
				delete(days, key)
			}
		}
		if len(days) == 0 {
			delete(c.rules, rule)
		}
	}
}

// windowStart is the first day, at midnight UTC, of the window ending at
// now.
func windowStart(now time.Time) time.Time {
	d := now.UTC().Truncate(24 * time.Hour)
	return d.AddDate(0, 0, -(Window - 1))
}

// Size is the number of stored day entries, for tests of the bound.
func (c *Counts) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, days := range c.rules {
		n += len(days)
	}
	return n
}

// RuleSummary is one rule's outcomes over the window.
type RuleSummary struct {
	Rule      string `json:"rule"`
	Confirmed int    `json:"confirmed"`
	Other     int    `json:"otherChanged"`
	None      int    `json:"nothingChanged"`
}

// Total is the number of resolved incidents counted.
func (r RuleSummary) Total() int { return r.Confirmed + r.Other + r.None }

// Sentence reads "cause confirmed by the fix in 3 of 4 resolved incidents
// (last 7 days)".
func (r RuleSummary) Sentence() string {
	noun := "incidents"
	if r.Total() == 1 {
		noun = "incident"
	}
	return "cause confirmed by the fix in " + strconv.Itoa(r.Confirmed) +
		" of " + strconv.Itoa(r.Total()) + " resolved " + noun +
		" (last " + strconv.Itoa(Window) + " days)"
}

// Summary totals the window ending at now, rules in name order.
func (c *Counts) Summary(now time.Time) []RuleSummary {
	c.mu.Lock()
	defer c.mu.Unlock()
	oldest := windowStart(now).Format(dayFormat)
	var out []RuleSummary
	for rule, days := range c.rules {
		s := RuleSummary{Rule: rule}
		for key, t := range days {
			if key < oldest {
				continue
			}
			s.Confirmed, s.Other, s.None = s.Confirmed+t[0],
				s.Other+t[1], s.None+t[2]
		}
		if s.Total() > 0 {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out
}

// MarshalJSON writes {rule: {day: [confirmed, other, none]}}.
func (c *Counts) MarshalJSON() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Marshal(c.rules)
}

// UnmarshalJSON reads what MarshalJSON wrote. Days that do not parse and
// rules past the bound are skipped, so a damaged value cannot grow the
// counts.
func (c *Counts) UnmarshalJSON(data []byte) error {
	var in map[string]map[string]tally
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = map[string]map[string]tally{}
	for rule, days := range in {
		if len(c.rules) >= MaxRules {
			break
		}
		for key, t := range days {
			if _, err := time.Parse(dayFormat, key); err != nil {
				continue
			}
			if c.rules[rule] == nil {
				c.rules[rule] = map[string]tally{}
			}
			c.rules[rule][key] = t
		}
	}
	return nil
}
