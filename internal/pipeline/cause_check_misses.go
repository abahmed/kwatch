package pipeline

import (
	"encoding/json"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/selfcheck"
	"github.com/abahmed/kwatch/internal/status"
)

// causeChecksVersion marks the saved value that holds the counts and the
// likely misses. An older kwatch saved the counts alone, as a map of
// rules; decode reads both.
const causeChecksVersion = 2

// savedChecks is the saved cause self-check.
type savedChecks struct {
	Version int               `json:"v"`
	Counts  *selfcheck.Counts `json:"counts"`
	Misses  []selfcheck.Miss  `json:"misses,omitempty"`
}

// encode is the value saved in the state bucket.
func (c *causeChecker) encode() (json.RawMessage, error) {
	return json.Marshal(savedChecks{Version: causeChecksVersion,
		Counts: c.counts, Misses: c.misses.Snapshot()})
}

// decode restores a saved value of either version; one it cannot read
// leaves the checker empty.
func (c *causeChecker) decode(raw json.RawMessage) {
	var saved savedChecks
	if json.Unmarshal(raw, &saved) == nil &&
		saved.Version == causeChecksVersion && saved.Counts != nil {
		c.counts = saved.Counts
		c.misses.Restore(saved.Misses)
		return
	}
	// The counts alone, as an older kwatch saved them.
	counts := selfcheck.NewCounts()
	if json.Unmarshal(raw, counts) == nil {
		c.counts = counts
	}
}

// missOf is the likely miss of inc, whose recovery followed other, a
// change to something other than the cause it named.
func missOf(inc incident.Incident, other inventory.Change) selfcheck.Miss {
	m := selfcheck.Miss{ID: inc.ID, Rule: inc.Cause.Rule,
		Root: inc.Cause.Root.String(), Opened: inc.Opened,
		Resolved: inc.Resolved}
	if !other.Entity.IsZero() {
		m.Other = other.Entity.String()
		m.Change = string(other.Classify())
	}
	return m
}

// exportMargin widens a likely miss's window for replay-export, so the
// export holds what led up to it and what followed.
const exportMargin = 10 * time.Minute

// missesReport is the likely misses as /status shows them, newest first.
func (c *causeChecker) missesReport(now time.Time) []status.LikelyMiss {
	var out []status.LikelyMiss
	for _, m := range c.misses.List(now) {
		out = append(out, status.LikelyMiss{ID: m.ID, Rule: m.Rule,
			Root: m.Root, Other: m.Other, Change: m.Change,
			Opened: m.Opened, Resolved: m.Resolved,
			ExportSince: m.Opened.Add(-exportMargin).UTC(),
			ExportUntil: m.Resolved.Add(exportMargin).UTC()})
	}
	return out
}
