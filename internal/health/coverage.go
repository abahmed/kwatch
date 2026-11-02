package health

import "sort"

// MaxCoverageKinds caps the unavailable kinds listed in /health.
const MaxCoverageKinds = 50

// Coverage reason codes are the complete bounded vocabulary. They are
// the source's own codes (inventory/kube), so /health and the
// dashboard's /api/coverage name a gap the same way.
const (
	CoveragePermissionDenied = "permission_denied"
	// CoverageAPIUnavailable is a kind the API does not serve, or one
	// kwatch cannot watch.
	CoverageAPIUnavailable = "api_unavailable"
	// CoverageSyncFailed, CoverageSyncTimeout and CoverageSyncPending
	// are kinds whose first list failed, took too long, or still runs:
	// kwatch's watch, not the cluster.
	CoverageSyncFailed  = "cache_sync_failed"
	CoverageSyncTimeout = "cache_sync_timeout"
	CoverageSyncPending = "cache_sync_pending"
	// CoverageHandlerPanics is a kind whose event handler panicked.
	CoverageHandlerPanics = "handler_panics"
	// CoverageBudgetExceeded is a kind left out by the watch budget.
	CoverageBudgetExceeded = "watch_budget_exceeded"
	// CoverageDisabledByConfig marks a kind configuration turned off,
	// such as Secrets with watch.secrets: false.
	CoverageDisabledByConfig = "disabled_by_config"
	// CoverageObjectCapReached marks a watched kind with more objects than
	// its cap; the extra objects are not observed.
	CoverageObjectCapReached = "object_cap_reached"
	// CoverageUnknown is a reason outside this vocabulary.
	CoverageUnknown = "unknown"
)

// legacyCoverageReasons are the names /health used before it shared
// the source's codes; they are still read, and written in the new name.
var legacyCoverageReasons = map[string]string{
	"sync_timeout":    CoverageSyncTimeout,
	"budget_exceeded": CoverageBudgetExceeded,
}

// CoverageKind is one resource type that is not being watched, or, with
// reason object_cap_reached, only partly observed.
type CoverageKind struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// CoverageSummary is the bounded watch coverage published in /health.
type CoverageSummary struct {
	// Watched counts running watches by mode (full, hashed, status,
	// metadata).
	Watched map[string]int `json:"watched,omitempty"`
	Skipped int            `json:"skipped"`
	// Unavailable is the total count; UnavailableKinds lists at most
	// MaxCoverageKinds of the unavailable, skipped and capped kinds,
	// sorted by kind.
	Unavailable      int            `json:"unavailable"`
	UnavailableKinds []CoverageKind `json:"unavailableKinds,omitempty"`
	Truncated        bool           `json:"truncated,omitempty"`
	// Reasons counts every unavailable, skipped and capped kind by
	// reason, including the ones the bounded list leaves out.
	Reasons map[string]int `json:"reasons,omitempty"`
	// Complete is false when the last discovery was partial; it is true
	// before a discovery ran, when nothing partial is known.
	Complete bool `json:"complete"`
	// Ran is set once a discovery finished. It is not published.
	Ran bool `json:"-"`
}

// SetCoverage publishes a detached, bounded copy of the summary. Unknown
// reasons collapse to unknown and kind names are not free text.
func (h *HealthServer) SetCoverage(summary CoverageSummary) {
	bounded := boundCoverage(summary)
	h.componentMu.Lock()
	defer h.componentMu.Unlock()
	h.coverage = &bounded
}

// Coverage returns a detached copy of the last published summary, or nil
// before the first publication.
func (h *HealthServer) Coverage() *CoverageSummary {
	h.componentMu.RLock()
	defer h.componentMu.RUnlock()
	if h.coverage == nil {
		return nil
	}
	out := boundCoverage(*h.coverage)
	return &out
}

func boundCoverage(in CoverageSummary) CoverageSummary {
	out := CoverageSummary{
		Skipped: in.Skipped, Unavailable: in.Unavailable,
		Complete: in.Complete, Truncated: in.Truncated, Ran: in.Ran,
	}
	if len(in.Watched) > 0 {
		out.Watched = make(map[string]int, len(in.Watched))
		for mode, n := range in.Watched {
			out.Watched[mode] = n
		}
	}
	if len(in.Reasons) > 0 {
		out.Reasons = make(map[string]int, len(in.Reasons))
		for reason, n := range in.Reasons {
			out.Reasons[coverageReason(reason)] += n
		}
	}
	kinds := append([]CoverageKind(nil), in.UnavailableKinds...)
	sort.Slice(kinds, func(i, j int) bool { return kinds[i].Kind < kinds[j].Kind })
	if len(kinds) > MaxCoverageKinds {
		kinds = kinds[:MaxCoverageKinds]
		out.Truncated = true
	}
	for i := range kinds {
		kinds[i].Reason = coverageReason(kinds[i].Reason)
	}
	out.UnavailableKinds = kinds
	return out
}

func coverageReason(reason string) string {
	switch reason {
	case CoveragePermissionDenied, CoverageAPIUnavailable,
		CoverageSyncFailed, CoverageSyncTimeout, CoverageSyncPending,
		CoverageHandlerPanics, CoverageBudgetExceeded,
		CoverageDisabledByConfig, CoverageObjectCapReached,
		CoverageUnknown:
		return reason
	}
	if current, ok := legacyCoverageReasons[reason]; ok {
		return current
	}
	return CoverageUnknown
}
