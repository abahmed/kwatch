package correlation

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

// flush using scope-adapted format.
func (e *Engine) buildGroupSummary(entries []groupEntry, firstSeen time.Time) string {
	if len(entries) == 0 {
		return ""
	}
	scope := detectGroupScope(entries)
	r := entries[0].reason
	timeAgo := ""
	if d := e.now().Sub(firstSeen).Round(time.Second); d > 0 {
		timeAgo = fmt.Sprintf(" — %s", timeAgoStr(d))
	}
	switch scope {
	case "node":
		return e.buildNodeSummary(r, entries, timeAgo)
	case "signature":
		return e.buildSignatureSummary(r, entries, timeAgo)
	case "image":
		return e.buildImageSummary(r, entries, timeAgo)
	case "owner":
		return e.buildOwnerSummary(r, entries, timeAgo)
	case "namespace":
		return e.buildNamespaceSummary(r, entries, timeAgo)
	default:
		return e.buildGenericSummary(r, entries, timeAgo)
	}
}

func timeAgoStr(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func pluralize(s string, n int) string {
	if n == 1 {
		return s
	}
	return s + "s"
}

func detectGroupScope(entries []groupEntry) string {
	r := entries[0].reason
	nodes := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.nodeName })
	if len(nodes) == 1 && isNodeLevelReason(r) {
		return "node"
	}
	sigs := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.logSignature })
	if len(sigs) == 1 {
		return "signature"
	}
	imgs := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.image })
	if len(imgs) == 1 {
		return "image"
	}
	owners := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.owner })
	if len(owners) == 1 {
		return "owner"
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
	return out
}

func (e *Engine) buildOwnerSummary(r string, entries []groupEntry, timeAgo string) string {
	byPod := make(map[string]int)
	for _, ge := range entries {
		byPod[ge.podName]++
	}
	podList := make([]string, 0, len(byPod))
	for p := range byPod {
		podList = append(podList, p)
	}
	sort.Strings(podList)
	owner := entries[0].namespace + "/" + entries[0].owner
	total := len(entries)
	return fmt.Sprintf("%s — %d %s in %s (total %d)%s",
		r, len(byPod), pluralize("pod", len(byPod)), owner, total, timeAgo)
}

func (e *Engine) buildNodeSummary(r string, entries []groupEntry, timeAgo string) string {
	node := entries[0].nodeName
	byPod := make(map[string]int)
	for _, ge := range entries {
		byPod[ge.podName]++
	}
	podCount := len(byPod)
	total := len(entries)
	return fmt.Sprintf("%s — node: %s, %d %s affected (total %d)%s",
		r, node, podCount, pluralize("pod", podCount), total, timeAgo)
}

func (e *Engine) buildSignatureSummary(r string, entries []groupEntry, timeAgo string) string {
	sig := entries[0].logSignature
	byOwner := make(map[string]int)
	for _, ge := range entries {
		o := ge.namespace + "/" + ge.owner
		byOwner[o]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	return fmt.Sprintf("%s — %s, %s: %s (total %d)%s",
		r, sig, pluralize("deployment", len(byOwner)), strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) buildImageSummary(r string, entries []groupEntry, timeAgo string) string {
	img := entries[0].image
	isGlobal := IsGlobalKey(entries[0].key)
	if img == "" {
		img = "unknown"
	}
	if isGlobal {
		byOwner := make(map[string]int)
		for _, ge := range entries {
			o := ge.namespace + "/" + ge.owner
			byOwner[o]++
		}
		ownerCount := len(byOwner)
		total := len(entries)
		return fmt.Sprintf("%s — %d %s affected (total %d)%s",
			r, ownerCount, pluralize("deployment", ownerCount), total, timeAgo)
	}
	byOwner := make(map[string]int)
	for _, ge := range entries {
		byOwner[ge.owner]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	ns := entries[0].namespace
	if ns != "" {
		ns = " (" + ns + ")"
	}
	return fmt.Sprintf("%s — image %q%s, %s: %s (total %d)%s",
		r, img, ns, pluralize("deployment", len(byOwner)), strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) buildNamespaceSummary(r string, entries []groupEntry, timeAgo string) string {
	ns := entries[0].namespace
	byOwner := make(map[string]int)
	for _, ge := range entries {
		byOwner[ge.owner]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	return fmt.Sprintf("%s — namespace: %s, %s: %s (total %d)%s",
		r, ns, pluralize("deployment", len(byOwner)), strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) buildGenericSummary(r string, entries []groupEntry, timeAgo string) string {
	byOwner := make(map[string]int)
	for _, ge := range entries {
		o := ge.namespace + "/" + ge.owner
		byOwner[o]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	return fmt.Sprintf("%s — affected: %s (total %d)%s",
		r, strings.Join(owners, ", "), total, timeAgo)
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
