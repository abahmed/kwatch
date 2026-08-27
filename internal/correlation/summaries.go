package correlation

import (
	"fmt"
	"sort"

	"github.com/abahmed/kwatch/internal/format"

	"github.com/abahmed/kwatch/internal/model"
)

// flush using scope-adapted format.
// buildGroupSummary names a group the way a person would describe it to a
// colleague. The reason, the count and the age are rendered as their own
// fields by every provider, so the summary must not repeat them — it used to
// read "ContainersNotReady — 1 pod in dev/api (total 1) — 1m",
// which said the reason twice and the size three times.
func (e *Engine) buildGroupSummary(entries []groupEntry) string {
	if len(entries) == 0 {
		return ""
	}
	switch detectGroupScope(entries) {
	case "node":
		return e.buildNodeSummary(entries)
	case "signature":
		return e.buildSignatureSummary(entries)
	case "image":
		return e.buildImageSummary(entries)
	case "owner":
		return e.buildOwnerSummary(entries)
	case "namespace":
		return e.buildNamespaceSummary(entries)
	default:
		return e.buildGenericSummary(entries)
	}
}

func detectGroupScope(entries []groupEntry) string {
	r := entries[0].reason
	nodes := uniqueNonEmptyStr(
		entries,
		func(e groupEntry) string { return e.nodeName },
	)
	if len(nodes) == 1 && isNodeLevelReason(r) {
		return "node"
	}
	sigs := uniqueNonEmptyStr(
		entries,
		func(e groupEntry) string { return e.logSignature },
	)
	if len(sigs) == 1 {
		return "signature"
	}
	imgs := uniqueNonEmptyStr(
		entries,
		func(e groupEntry) string { return e.image },
	)
	if len(imgs) == 1 {
		return "image"
	}
	owners := uniqueNonEmptyStr(
		entries,
		func(e groupEntry) string { return e.owner },
	)
	if len(owners) == 1 {
		return "owner"
	}
	namespaces := uniqueNonEmptyStr(
		entries,
		func(e groupEntry) string { return e.namespace },
	)
	if len(namespaces) == 1 {
		return "namespace"
	}
	return "generic"
}

func uniqueNonEmptyStr[T any](entries []T, fn func(T) string) []string {
	m := make(map[string]bool)
	for _, e := range entries {
		if v := fn(e); v != "" {
			m[v] = true
		}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// ownerRefs lists the distinct owners, as "name" within one namespace or
// "ns/name" across several, with a multiplier when one owner has several
// failing pods.
func ownerRefs(entries []groupEntry, withNamespace bool) []string {
	counts := make(map[string]int)
	for _, ge := range entries {
		ref := ge.owner
		if withNamespace && ge.namespace != "" {
			ref = ge.namespace + "/" + ge.owner
		}
		if ref == "" {
			continue
		}
		counts[ref]++
	}
	out := make([]string, 0, len(counts))
	for ref, n := range counts {
		if n > 1 {
			out = append(out, fmt.Sprintf("%s ×%d", ref, n))
		} else {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

func podCount(entries []groupEntry) int {
	return len(
		uniqueNonEmptyStr(
			entries,
			func(e groupEntry) string { return e.podName },
		),
	)
}

// "3 pods of Deployment dev/api"
func (e *Engine) buildOwnerSummary(entries []groupEntry) string {
	first := entries[0]
	owner := first.owner
	if first.namespace != "" {
		owner = first.namespace + "/" + owner
	}
	kind := ""
	if inc, ok := e.state[first.key]; ok && inc.OwnerKind != "" {
		kind = inc.OwnerKind + " "
	}
	return fmt.Sprintf(
		"%s of %s%s",
		format.Plural(podCount(entries), "pod"),
		kind,
		owner,
	)
}

// "12 pods on node ip-10-0-81-7 across 6 workloads"
func (e *Engine) buildNodeSummary(entries []groupEntry) string {
	owners := uniqueNonEmptyStr(
		entries,
		func(e groupEntry) string { return e.namespace + "/" + e.owner },
	)
	s := fmt.Sprintf(
		"%s on node %s",
		format.Plural(podCount(entries), "pod"),
		format.ShortNode(entries[0].nodeName),
	)
	if len(owners) > 1 {
		s += fmt.Sprintf(" across %s", format.Plural(len(owners), "workload"))
	}
	return s
}

// "api, readify, tracking — same error: connection refused:5432"
func (e *Engine) buildSignatureSummary(entries []groupEntry) string {
	return fmt.Sprintf("%s — same error: %s",
		format.JoinNames(ownerRefs(entries, true), 5), entries[0].logSignature)
}

// "image api:1.2.0 — api, api-worker"  or, for a
// registry-wide failure, "4 workloads across 3 namespaces"
func (e *Engine) buildImageSummary(entries []groupEntry) string {
	if IsGlobalKey(entries[0].key) {
		owners := uniqueNonEmptyStr(
			entries,
			func(e groupEntry) string { return e.namespace + "/" + e.owner },
		)
		namespaces := uniqueNonEmptyStr(
			entries,
			func(e groupEntry) string { return e.namespace },
		)
		s := format.Plural(len(owners), "workload")
		if len(namespaces) > 1 {
			s += fmt.Sprintf(
				" across %s",
				format.Plural(len(namespaces), "namespace"),
			)
		}
		return s
	}
	img := format.ShortImage(entries[0].image)
	if img == "" {
		img = "unknown image"
	} else {
		img = "image " + img
	}
	return fmt.Sprintf(
		"%s — %s",
		img,
		format.JoinNames(ownerRefs(entries, false), 5),
	)
}

// "6 workloads in dev: accounts, api, fleet, readify, tdesk,
// tracking"
func (e *Engine) buildNamespaceSummary(entries []groupEntry) string {
	owners := ownerRefs(entries, false)
	return fmt.Sprintf(
		"%s in %s: %s",
		format.Plural(
			len(owners),
			"workload",
		),
		entries[0].namespace,
		format.JoinNames(owners, 8),
	)
}

// "ns-a/api, ns-b/worker"
func (e *Engine) buildGenericSummary(entries []groupEntry) string {
	return format.JoinNames(ownerRefs(entries, true), 8)
}

func (e *Engine) groupSeverity(entries []groupEntry) model.Severity {
	best := model.SeverityNormal
	for _, ge := range entries {
		if inc, ok := e.state[ge.key]; ok {
			if inc.Severity.Rank() > best.Rank() {
				best = inc.Severity
			}
		}
	}
	return best
}
