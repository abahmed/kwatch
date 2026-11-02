package kube

import (
	"sort"
	"time"
)

// MaxCoverageNames bounds every list of names in a Coverage, so a cluster
// with thousands of namespaces or custom kinds cannot grow the response.
const MaxCoverageNames = 50

// Coverage says what kwatch watches: the namespaces in its scope, the
// kinds it watches and has synced, the kinds it cannot read and why, and
// when the sources finished their first sync. Every list is sorted and
// bounded by MaxCoverageNames; the counts include what a list leaves
// out.
type Coverage struct {
	Namespaces NamespaceCoverage `json:"namespaces"`
	Kinds      KindCoverage      `json:"kinds"`
	// SyncedAt is when every kind first finished its initial list (or
	// gave up on it); zero while the sources are still listing.
	SyncedAt time.Time `json:"syncedAt,omitzero"`
	// DiscoveryComplete is false when the last discovery of the API's
	// kinds was partial.
	DiscoveryComplete bool `json:"discoveryComplete"`
}

// NamespaceCoverage splits the cluster's namespaces by the namespace
// scope (namespaces, excluded namespaces and the namespace selector).
type NamespaceCoverage struct {
	// All is set when no namespace list or selector narrows the scope.
	All           bool     `json:"all"`
	Watched       int      `json:"watched"`
	WatchedNames  []string `json:"watchedNames"`
	Excluded      int      `json:"excluded"`
	ExcludedNames []string `json:"excludedNames"`
	Truncated     bool     `json:"truncated,omitempty"`
}

// KindCoverage is the watch state of every kind kwatch reads.
type KindCoverage struct {
	// Watched counts running watches, typed and discovered; Synced the
	// ones whose cache holds the whole list.
	Watched int            `json:"watched"`
	Synced  int            `json:"synced"`
	ByMode  map[string]int `json:"byMode"`
	// Typed lists the built-in kinds kwatch always watches.
	Typed []WatchedKind `json:"typed"`
	// Unreadable lists kinds the API refused to kwatch (RBAC).
	Unreadable []KindGap `json:"unreadable"`
	// Unsynced lists kinds whose first list has not finished.
	Unsynced []KindGap `json:"unsynced"`
	// NotWatched lists kinds turned off by configuration, missing from
	// the API, over the watch budget, or over their object cap.
	NotWatched []KindGap `json:"notWatched"`
	// Reasons counts every gap by reason, the ones the lists leave out
	// included.
	Reasons   map[string]int `json:"reasons,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}

// WatchedKind is one typed watch: its resource ("pods",
// "deployments.apps"), watch mode and whether it has synced.
type WatchedKind struct {
	Kind     string `json:"kind"`
	Mode     string `json:"mode"`
	Required bool   `json:"required,omitempty"`
	Synced   bool   `json:"synced"`
}

// KindGap is one kind kwatch does not fully read, with the reason code
// and a short explanation.
type KindGap struct {
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail"`
	Required bool   `json:"required,omitempty"`
}

// CoverageInput is what BuildCoverage summarises.
type CoverageInput struct {
	// Typed, Unavailable and Disabled come from the typed Source.
	Typed       []WatchedKind
	Unavailable []SourceStatus
	Disabled    []SourceStatus
	Dynamic     DynamicStatus
	SyncedAt    time.Time
	// Namespaces are the namespaces the model holds; InScope reports
	// whether one is in the namespace scope, and All whether nothing
	// narrows that scope.
	Namespaces []string
	InScope    func(namespace string) bool
	All        bool
}

// SyncedAt is when WaitForSync first saw every kind settle; zero before.
func (s *Source) SyncedAt() time.Time {
	if at := s.syncedAt.Load(); at != 0 {
		return time.Unix(0, at)
	}
	return time.Time{}
}

// WatchedKinds lists the typed watches, sorted by kind.
func (s *Source) WatchedKinds() []WatchedKind {
	modes := map[Resource]WatchMode{}
	for _, r := range registrations() {
		modes[r.resource] = r.mode()
	}
	out := make([]WatchedKind, 0, len(s.watches))
	for _, w := range s.watches {
		mode := modes[w.resource]
		if mode == "" {
			mode = WatchFull
		}
		out = append(out, WatchedKind{
			Kind: resourceKey(w.resource.Group, w.resource.Name),
			Mode: string(mode), Required: w.required,
			Synced: w.hasSynced() && !w.accessLost(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// BuildCoverage summarises in.
func BuildCoverage(in CoverageInput) Coverage {
	return Coverage{
		Namespaces:        namespaceCoverage(in),
		Kinds:             kindCoverage(in),
		SyncedAt:          in.SyncedAt,
		DiscoveryComplete: !in.Dynamic.Ran || in.Dynamic.Complete,
	}
}

func namespaceCoverage(in CoverageInput) NamespaceCoverage {
	var watched, excluded []string
	for _, ns := range in.Namespaces {
		if in.InScope == nil || in.InScope(ns) {
			watched = append(watched, ns)
		} else {
			excluded = append(excluded, ns)
		}
	}
	out := NamespaceCoverage{
		All: in.All, Watched: len(watched), Excluded: len(excluded),
	}
	var cut1, cut2 bool
	out.WatchedNames, cut1 = boundNames(watched)
	out.ExcludedNames, cut2 = boundNames(excluded)
	out.Truncated = cut1 || cut2
	return out
}

// boundNames sorts names and keeps the first MaxCoverageNames.
func boundNames(names []string) ([]string, bool) {
	out := append([]string{}, names...)
	sort.Strings(out)
	if len(out) > MaxCoverageNames {
		return out[:MaxCoverageNames], true
	}
	return out, false
}

func kindCoverage(in CoverageInput) KindCoverage {
	out := KindCoverage{
		ByMode: map[string]int{}, Typed: in.Typed,
		Unreadable: []KindGap{}, Unsynced: []KindGap{},
		NotWatched: []KindGap{}, Reasons: map[string]int{},
	}
	refused := refusedKinds(in.Unavailable)
	for _, k := range in.Typed {
		if refused[k.Kind] {
			// Listed once, as unreadable: a watch the API refuses
			// reads nothing.
			continue
		}
		out.Watched++
		out.ByMode[k.Mode]++
		if k.Synced {
			out.Synced++
		}
	}
	for mode, n := range in.Dynamic.Watched {
		out.Watched += n
		out.ByMode[string(mode)] += n
	}
	out.Synced += in.Dynamic.Synced
	for _, gap := range kindGaps(in) {
		out.add(gap)
	}
	for _, s := range in.Unavailable {
		out.Reasons[s.Reason]++
	}
	for _, s := range in.Disabled {
		out.Reasons[s.Reason]++
	}
	for reason, n := range in.Dynamic.Reasons {
		out.Reasons[reason] += n
	}
	out.Truncated = out.Truncated || in.Dynamic.KindsTruncated
	if len(out.Reasons) == 0 {
		out.Reasons = nil
	}
	return out
}

// refusedKinds are the typed kinds the API refuses to kwatch, by
// resource key.
func refusedKinds(unavailable []SourceStatus) map[string]bool {
	out := map[string]bool{}
	for _, s := range unavailable {
		if s.Reason == ReasonPermissionDenied {
			out[resourceKey(s.Group, s.Resource)] = true
		}
	}
	return out
}

// kindGaps lists every typed and discovered kind that is not fully read,
// sorted by kind. The discovered list is already bounded; the dynamic
// source counts what it left out by reason.
func kindGaps(in CoverageInput) []KindGap {
	var gaps []KindGap
	typed := append(append([]SourceStatus{}, in.Unavailable...),
		in.Disabled...)
	for _, s := range typed {
		gaps = append(gaps, KindGap{
			Kind: resourceKey(s.Group, s.Resource), Reason: s.Reason,
			Detail: gapDetail(s.Reason), Required: s.Required,
		})
	}
	for _, k := range in.Dynamic.Kinds {
		gaps = append(gaps, KindGap{
			Kind: k.Resource, Reason: k.Reason, Detail: gapDetail(k.Reason),
		})
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Kind != gaps[j].Kind {
			return gaps[i].Kind < gaps[j].Kind
		}
		return gaps[i].Reason < gaps[j].Reason
	})
	return gaps
}

// add files gap under its list, keeping each list bounded.
func (c *KindCoverage) add(gap KindGap) {
	list := &c.NotWatched
	switch gap.Reason {
	case ReasonPermissionDenied:
		list = &c.Unreadable
	case ReasonSyncPending, ReasonSyncTimeout, ReasonSyncFailed:
		list = &c.Unsynced
	}
	if len(*list) == MaxCoverageNames {
		c.Truncated = true
		return
	}
	*list = append(*list, gap)
}

// resourceKey names a resource "resource.group", or "resource" in the
// core group.
func resourceKey(group, name string) string {
	if group == "" {
		return name
	}
	return name + "." + group
}

// gapDetails explain each reason code in a sentence.
var gapDetails = map[string]string{
	ReasonPermissionDenied: "the API refused to list or watch it: " +
		"kwatch's service account lacks the RBAC grant",
	ReasonAPIUnavailable: "the API does not serve it, or kwatch " +
		"cannot watch this type",
	ReasonSyncFailed:       "its first list failed; kwatch retries",
	ReasonSyncTimeout:      "its first list has not finished in time",
	ReasonSyncPending:      "its first list is still running",
	ReasonDisabledByConfig: "configuration turned its watch off",
	ReasonWatchBudget: "left out: the cluster has more kinds than " +
		"the watch budget",
	ReasonObjectCap: "watched, but objects over its cap are " +
		"not observed",
}

func gapDetail(reason string) string {
	if detail, ok := gapDetails[reason]; ok {
		return detail
	}
	return gapDetails[ReasonAPIUnavailable]
}
