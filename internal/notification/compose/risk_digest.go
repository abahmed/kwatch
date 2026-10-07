package compose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// maxRiskExamples bounds how many workloads one risk type names.
const maxRiskExamples = 3

// neverReady keeps the findings the digest announces beside its
// problems: pods that run but never become ready while others serve.
// That is happening now, system namespaces included. Configuration
// advice (no probe, a single replica) is never announced; a failure
// quotes it only as a consequence.
func neverReady(findings []detection.Finding) []detection.Finding {
	var out []detection.Finding
	for _, f := range findings {
		if f.Reason == reasons.WorkloadNeverReady {
			out = append(out, f)
		}
	}
	return out
}

// riskWorkloads counts the distinct workloads with pods never ready.
func riskWorkloads(risks []detection.Finding) int {
	seen := map[inventory.EntityID]bool{}
	for _, f := range risks {
		seen[f.Entity] = true
	}
	return len(seen)
}

// riskGroup is every workload that has one type of finding.
type riskGroup struct {
	reason string
	// clause words the finding for one workload.
	clause string
	names  []string
	// ids are the same workloads, for lists that tell namespaces apart.
	ids []inventory.EntityID
}

// riskTitles names the workloads with pods that never become ready, with
// a few examples, in one sentence: "Running but not ready: 2 workloads
// have pods that run but never become ready (api, cart)."
func riskTitles(risks []detection.Finding) []sentence {
	groups := groupRisks(neverReady(risks))
	if len(groups) == 0 {
		return nil
	}
	clauses := make([]string, 0, len(groups))
	for _, g := range groups {
		clauses = append(clauses, riskGroupText(g))
	}
	return []sentence{{part: partProof, text: "Running but not ready: " +
		strings.Join(clauses, "; ") + "."}}
}

// groupRisks groups the risks by type, the most common type first.
func groupRisks(risks []detection.Finding) []riskGroup {
	byReason := map[string]*riskGroup{}
	seen := map[detection.Key]bool{}
	for _, f := range risks {
		if seen[f.Key()] {
			continue
		}
		seen[f.Key()] = true
		g := byReason[f.Reason]
		if g == nil {
			g = &riskGroup{reason: f.Reason, clause: riskClause(f)}
			byReason[f.Reason] = g
		}
		g.names = append(g.names, shortName(f.Entity))
		g.ids = append(g.ids, f.Entity)
	}
	out := make([]riskGroup, 0, len(byReason))
	for _, g := range byReason {
		sort.Strings(g.names)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].names) != len(out[j].names) {
			return len(out[i].names) > len(out[j].names)
		}
		return out[i].reason < out[j].reason
	})
	return out
}

// riskGroupText is "40 workloads have no readiness probe (orders,
// payments, cart and 37 more)".
func riskGroupText(g riskGroup) string {
	n := len(g.names)
	text := "1 workload " + g.clause
	if n > 1 {
		text = fmt.Sprintf("%d workloads %s", n, pluralPredicate(g.clause))
	}
	shown := limit(g.names, maxRiskExamples)
	return text + " (" + strings.Join(shown, ", ") + more(n, len(shown)) + ")"
}

// riskClause words what the workload does: its pods run but never
// become ready. Some replicas serve and the rest fail their readiness
// probe; an unknown reason falls back to its summary as a predicate.
func riskClause(f detection.Finding) string {
	if f.Reason == reasons.WorkloadNeverReady {
		return "has pods that run but never become ready"
	}
	return predicate(f.Entity, f.Summary)
}
