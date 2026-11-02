package app

import (
	"sort"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// coverageSink receives the bounded coverage summary.
type coverageSink interface {
	SetCoverage(health.CoverageSummary)
}

// dynamicCoverage is the dynamic source as coverage reporting sees it.
type dynamicCoverage interface {
	Status() kube.DynamicStatus
}

// buildCoverage merges typed sync state with the dynamic watch plan. Both
// list their affected kinds (the dynamic list is already bounded) and
// count them by reason, so /health shows why a kind is not observed even
// when the bounded list is truncated.
func buildCoverage(
	typed []kube.SourceStatus, dynamic kube.DynamicStatus,
) health.CoverageSummary {
	watched := map[string]int{}
	for mode, n := range kube.TypedWatched(typed) {
		watched[string(mode)] += n
	}
	for mode, n := range dynamic.Watched {
		watched[string(mode)] += n
	}
	reasons := map[string]int{}
	kinds := make([]health.CoverageKind, 0, len(typed)+len(dynamic.Kinds))
	for _, s := range typed {
		kind := s.Resource
		if s.Group != "" {
			kind += "." + s.Group
		}
		reason := coverageReason(s.Reason)
		reasons[reason]++
		kinds = append(kinds, health.CoverageKind{Kind: kind, Reason: reason})
	}
	for reason, n := range dynamic.Reasons {
		reasons[coverageReason(reason)] += n
	}
	for _, k := range dynamic.Kinds {
		kinds = append(kinds, health.CoverageKind{
			Kind: k.Resource, Reason: coverageReason(k.Reason),
		})
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i].Kind < kinds[j].Kind
	})
	summary := health.CoverageSummary{
		Watched:          watched,
		Skipped:          dynamic.Skipped,
		Unavailable:      len(typed) + dynamic.Unavailable,
		UnavailableKinds: kinds,
		Truncated:        dynamic.KindsTruncated,
		// Before a discovery ran (or with discovery off) there is no
		// partial discovery to report, as in kube.BuildCoverage.
		Complete: !dynamic.Ran || dynamic.Complete,
		Ran:      dynamic.Ran,
	}
	if len(reasons) > 0 {
		summary.Reasons = reasons
	}
	if len(kinds) > health.MaxCoverageKinds {
		summary.UnavailableKinds = kinds[:health.MaxCoverageKinds]
		summary.Truncated = true
	}
	return summary
}

// coverageReasons map every source reason code onto the coverage
// vocabulary, which uses the same names. A code missing here reads as
// unknown, never as a kind the API does not serve.
var coverageReasons = map[string]string{
	kube.ReasonPermissionDenied: health.CoveragePermissionDenied,
	kube.ReasonAPIUnavailable:   health.CoverageAPIUnavailable,
	kube.ReasonSyncFailed:       health.CoverageSyncFailed,
	kube.ReasonSyncTimeout:      health.CoverageSyncTimeout,
	kube.ReasonSyncPending:      health.CoverageSyncPending,
	kube.ReasonHandlerPanics:    health.CoverageHandlerPanics,
	kube.ReasonDisabledByConfig: health.CoverageDisabledByConfig,
	kube.ReasonWatchBudget:      health.CoverageBudgetExceeded,
	kube.ReasonObjectCap:        health.CoverageObjectCapReached,
}

// coverageReason maps a source reason code onto the coverage
// vocabulary.
func coverageReason(reason string) string {
	if mapped, ok := coverageReasons[reason]; ok {
		return mapped
	}
	return health.CoverageUnknown
}

// coveragePublisher publishes the summary from typed and dynamic sources.
type coveragePublisher struct {
	sink    coverageSink
	typed   sourceAvailability
	dynamic dynamicCoverage
}

// disabledSources is the optional part of the typed source that lists
// kinds configuration turned off.
type disabledSources interface {
	Disabled() []kube.SourceStatus
}

// publish sends the summary to the sink and returns it. ok is false when
// nothing was published.
func (p coveragePublisher) publish() (
	summary health.CoverageSummary, ok bool,
) {
	if p.sink == nil || p.typed == nil || p.dynamic == nil {
		return summary, false
	}
	typed := p.typed.Unavailable()
	// A disabled kind is listed as not watched, with its own reason, but
	// it is not a source failure and never degrades health.
	if d, ok := p.typed.(disabledSources); ok {
		typed = append(typed, d.Disabled()...)
	}
	summary = buildCoverage(typed, p.dynamic.Status())
	p.sink.SetCoverage(summary)
	return summary, true
}
