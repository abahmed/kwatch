package compose

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// failedSpan is how long the incident's own failure lasted. A Warning
// event of the root (a node crossing an eviction threshold) stays a
// finding for the whole event window after the condition behind it is
// gone, and the pods it evicted linger as members: the span ends when
// the last failure that is not such a lingering event cleared. An
// incident that moved to another root counts only the failures of that
// root (see movedSpan).
func failedSpan(p incident.Incident) time.Duration {
	if own, ok := movedSpan(p); ok {
		return own
	}
	whole := p.FailedFor()
	if p.Cause == nil {
		return whole
	}
	lingering := lingeringSummaries(p.Cause.RootFindings)
	if len(lingering) == 0 {
		return whole
	}
	var end time.Time
	for _, e := range p.Timeline {
		if e.Entity == nil || *e.Entity != p.Root ||
			!strings.HasPrefix(e.Text, incident.RecoveredPrefix) ||
			mentionsAny(e.Text, lingering) {
			continue
		}
		if e.At.After(end) {
			end = e.At
		}
	}
	if own := end.Sub(p.Opened); !end.IsZero() && own > 0 && own < whole {
		return own
	}
	return whole
}

// lingeringSummaries are the summaries of root findings that come from
// a Warning event rather than from a state.
func lingeringSummaries(found []detection.Finding) []string {
	var out []string
	for _, f := range found {
		if _, ok := detection.EventSeverity(f.Reason); ok &&
			f.Summary != "" {
			out = append(out, f.Summary)
		}
	}
	return out
}

// mentionsAny reports text that contains one of the summaries.
func mentionsAny(text string, summaries []string) bool {
	for _, s := range summaries {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}
