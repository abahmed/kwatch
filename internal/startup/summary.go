package startup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

const maxSummaryReasons = 8

// BuildSummary creates the optional startup report from baseline findings.
// Startup owns this presentation because it is an application lifecycle
// concern, not a Kubernetes resource handler concern.
func BuildSummary(enabled bool, suppressed map[string]int) *model.Incident {
	if !enabled || len(suppressed) == 0 {
		return nil
	}
	hint, total := summaryHint(suppressed)
	return &model.Incident{
		Subject: model.Subject{
			ID:     "startup-baseline",
			Key:    "startup:baseline",
			Reason: constant.ReasonPreExistingAtStartup,
		},
		Status: model.Status{
			Severity: model.SeverityNormal,
			Count:    total,
		},
		Evidence: model.Evidence{Hint: hint},
	}
}

// summaryHint condenses baseline findings into reason counts and workload
// count. It intentionally omits individual workload names from the alert.
func summaryHint(suppressed map[string]int) (string, int) {
	byReason := make(map[string]int)
	owners := make(map[string]bool)
	total := 0
	for key, count := range suppressed {
		total += count
		reason, owner := splitSummaryKey(key)
		byReason[reason] += count
		if owner != "" {
			owners[owner] = true
		}
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if byReason[reasons[i]] != byReason[reasons[j]] {
			return byReason[reasons[i]] > byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, maxSummaryReasons+1)
	for i, reason := range reasons {
		if i == maxSummaryReasons {
			parts = append(parts,
				fmt.Sprintf("+%d other kinds", len(reasons)-i))
			break
		}
		parts = append(parts, fmt.Sprintf(
			"%s ×%d", reason, byReason[reason],
		))
	}
	hint := fmt.Sprintf(
		"kwatch started with %d pre-existing issue(s), not re-alerted",
		total,
	)
	if len(owners) > 0 {
		hint += fmt.Sprintf(" across %d workloads", len(owners))
	}
	return hint + ": " + strings.Join(parts, ", "), total
}

func splitSummaryKey(key string) (reason, owner string) {
	reason = key
	if i := strings.LastIndex(key, "/"); i >= 0 {
		reason, owner = key[i+1:], key[:i]
	}
	return reason, owner
}
