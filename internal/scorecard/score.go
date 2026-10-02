package scorecard

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
)

// Report is the replayed notification quality of one audit log.
type Report struct {
	Window         time.Duration `json:"window"`
	Notifications  int           `json:"notifications"`
	PerHour        float64       `json:"perHour"`
	Incidents      int           `json:"incidents"`
	PerIncident    float64       `json:"perIncident"`
	PerIncidentP95 int           `json:"perIncidentP95"`
	// MaxPerIncident is the most messages any one incident produced.
	MaxPerIncident int `json:"maxPerIncident"`
	// PeakPerHour is the most notifications inside any one-hour window.
	PeakPerHour      int           `json:"peakPerHour"`
	Updates          int           `json:"updates"`
	UnchangedUpdates int           `json:"unchangedUpdates"`
	Recreated        int           `json:"recreated"`
	RepeatedResolves int           `json:"repeatedResolves"`
	Grouped          int           `json:"grouped"`
	UnknownCause     int           `json:"unknownCause"`
	CircularCause    int           `json:"circularCause"`
	TopReasons       []ReasonCount `json:"topReasons"`
}

// ReasonCount is the notification volume of one reason.
type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// UnchangedUpdatePercent is the share of updates that repeated the previous
// message for the same incident.
func (r Report) UnchangedUpdatePercent() float64 {
	return percent(r.UnchangedUpdates, r.Updates)
}

// RecreatedPercent is the share of incidents that were announced again
// after they resolved, either under the same ID or as a new incident
// linked to the resolved one.
func (r Report) RecreatedPercent() float64 {
	return percent(r.Recreated, r.Incidents)
}

// UnknownCausePercent is the share of notifications without a cause.
func (r Report) UnknownCausePercent() float64 {
	return percent(r.UnknownCause, r.Notifications)
}

type keyState struct {
	messages int
	lastHash string
	lastSent audit.Action
	resolved bool
}

// Score replays entries, which must be in time order.
func Score(entries []audit.Entry) Report {
	var report Report
	keys := make(map[string]*keyState)
	reasons := make(map[string]int)
	var first, last time.Time
	times := make([]time.Time, 0, len(entries))
	for _, entry := range entries {
		times = append(times, entry.Timestamp)
		if first.IsZero() {
			first = entry.Timestamp
		}
		last = entry.Timestamp
		state := keys[entry.Incident]
		if state == nil {
			state = &keyState{}
			keys[entry.Incident] = state
		}
		report.Notifications++
		reasons[entry.Reason]++
		scoreEntry(&report, state, entry)
	}
	report.Window = last.Sub(first)
	if hours := report.Window.Hours(); hours > 0 {
		report.PerHour = float64(report.Notifications) / hours
	}
	report.Incidents = len(keys)
	report.PerIncident, report.PerIncidentP95 = perIncident(keys)
	report.MaxPerIncident = maxPerIncident(keys)
	report.PeakPerHour = PeakInWindow(times, time.Hour)
	report.TopReasons = topReasons(reasons, 10)
	return report
}

func scoreEntry(report *Report, state *keyState, entry audit.Entry) {
	switch entry.Action {
	case audit.ActionCreate:
		if state.resolved || entry.Previous != "" {
			report.Recreated++
		}
		state.resolved = false
	case audit.ActionUpdate:
		report.Updates++
		if entry.ContentHash != "" && entry.ContentHash == state.lastHash {
			report.UnchangedUpdates++
		}
	case audit.ActionResolved:
		if state.lastSent == audit.ActionResolved {
			report.RepeatedResolves++
		}
		state.resolved = true
	}
	if entry.AffectedCount > 1 {
		report.Grouped++
	}
	if entry.Action != audit.ActionResolved {
		switch entry.CauseState {
		case "", audit.CauseUnknown:
			report.UnknownCause++
		case audit.CauseSelf:
			report.CircularCause++
		}
	}
	state.messages++
	state.lastSent = entry.Action
	if entry.ContentHash != "" {
		state.lastHash = entry.ContentHash
	}
}

func perIncident(keys map[string]*keyState) (float64, int) {
	if len(keys) == 0 {
		return 0, 0
	}
	counts := make([]int, 0, len(keys))
	total := 0
	for _, state := range keys {
		counts = append(counts, state.messages)
		total += state.messages
	}
	sort.Ints(counts)
	p95 := counts[(len(counts)*95+99)/100-1]
	return float64(total) / float64(len(keys)), p95
}

func maxPerIncident(keys map[string]*keyState) int {
	most := 0
	for _, state := range keys {
		most = max(most, state.messages)
	}
	return most
}

func topReasons(reasons map[string]int, limit int) []ReasonCount {
	out := make([]ReasonCount, 0, len(reasons))
	for reason, count := range reasons {
		out = append(out, ReasonCount{Reason: reason, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Reason < out[j].Reason
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func percent(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) * 100 / float64(whole)
}
