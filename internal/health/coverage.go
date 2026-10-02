package health

import "sort"

// MaxCoverageKinds caps the unavailable kinds listed in /health.
const MaxCoverageKinds = 50

// Coverage reason codes are the complete bounded vocabulary.
const (
	CoveragePermissionDenied = "permission_denied"
	CoverageAPIUnavailable   = "api_unavailable"
	CoverageSyncTimeout      = "sync_timeout"
	CoverageBudgetExceeded   = "budget_exceeded"
	// CoverageDisabledByConfig marks a kind configuration turned off,
	// such as Secrets with watch.secrets: false.
	CoverageDisabledByConfig = "disabled_by_config"
	// CoverageObjectCapReached marks a watched kind with more objects than
	// its cap; the extra objects are not observed.
	CoverageObjectCapReached = "object_cap_reached"
)

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
	// Complete is false when the last discovery was partial.
	Complete bool `json:"complete"`
}

// SetCoverage publishes a detached, bounded copy of the summary. Unknown
// reasons collapse to api_unavailable and kind names are not free text.
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
		Complete: in.Complete, Truncated: in.Truncated,
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
		CoverageSyncTimeout, CoverageBudgetExceeded,
		CoverageDisabledByConfig, CoverageObjectCapReached:
		return reason
	}
	return CoverageAPIUnavailable
}
