package scorecard

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
)

// Report is the replayed notification quality of one audit log.
type Report struct {
	Window           time.Duration `json:"window"`
	Notifications    int           `json:"notifications"`
	PerHour          float64       `json:"perHour"`
	Problems         int           `json:"problems"`
	PerProblem       float64       `json:"perProblem"`
	PerProblemP95    int           `json:"perProblemP95"`
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
// message for the same problem.
func (r Report) UnchangedUpdatePercent() float64 {
	return percent(r.UnchangedUpdates, r.Updates)
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
	for _, entry := range entries {
		if first.IsZero() {
			first = entry.Timestamp
		}
		last = entry.Timestamp
		state := keys[entry.Problem]
		if state == nil {
			state = &keyState{}
			keys[entry.Problem] = state
		}
		report.Notifications++
		reasons[entry.Reason]++
		scoreEntry(&report, state, entry)
	}
	report.Window = last.Sub(first)
	if hours := report.Window.Hours(); hours > 0 {
		report.PerHour = float64(report.Notifications) / hours
	}
	report.Problems = len(keys)
	report.PerProblem, report.PerProblemP95 = perProblem(keys)
	report.TopReasons = topReasons(reasons, 10)
	return report
}

func scoreEntry(report *Report, state *keyState, entry audit.Entry) {
	switch entry.Action {
	case audit.ActionCreate:
		if state.resolved {
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

func perProblem(keys map[string]*keyState) (float64, int) {
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
