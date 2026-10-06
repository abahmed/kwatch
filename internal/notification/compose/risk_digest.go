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

// withoutSystemRisks drops the risks of workloads in system
// namespaces: they are the cluster's, not the team's, to fix. Pods that
// run but never become ready are not a configuration choice but a state
// that is happening now, so a system workload's are kept.
func withoutSystemRisks(risks []detection.Finding) []detection.Finding {
	var out []detection.Finding
	for _, f := range risks {
		if !detection.SystemNamespace(f.Entity.Namespace) ||
			f.Reason == reasons.WorkloadNeverReady {
			out = append(out, f)
		}
	}
	return out
}

// riskWorkloads counts the distinct workloads that have a risk.
func riskWorkloads(risks []detection.Finding) int {
	seen := map[inventory.EntityID]bool{}
	for _, f := range risks {
		seen[f.Entity] = true
	}
	return len(seen)
}

// riskGroup is every workload that has one type of risk.
type riskGroup struct {
	reason string
	// clause words the risk for one workload: "runs a single replica".
	clause string
	names  []string
	// ids are the same workloads, for lists that tell namespaces apart.
	ids []inventory.EntityID
}

// riskTitles summarises the risks by type, with a few example
// workloads each, in one sentence: "Configuration risks: 40 workloads
// have no readiness probe (orders, payments, cart and 37 more); one
// workload runs a single replica (api)."
func riskTitles(risks []detection.Finding) []sentence {
	groups := groupRisks(withoutSystemRisks(risks))
	if len(groups) == 0 {
		return nil
	}
	clauses := make([]string, 0, len(groups))
	for _, g := range groups {
		clauses = append(clauses, riskGroupText(g))
	}
	return []sentence{{part: partProof, text: "Configuration risks: " +
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

// riskClauses word each configuration risk as what the workload does.
var riskClauses = map[string]string{
	reasons.RiskNoReadinessProbe: "has no readiness probe",
	reasons.RiskNoMemoryLimit:    "has containers without a memory limit",
	reasons.RiskMutableImageTag:  "runs an image tag that can change",
	reasons.RiskSingleReplica:    "runs a single replica",
	reasons.RiskSingleNode:       "runs every replica on one node",
	reasons.RiskPrivileged:       "runs a privileged container",
	// Some replicas serve and the rest run but fail their readiness
	// probe: not a configuration risk, but worth the digest.
	reasons.WorkloadNeverReady: "has pods that run but never become ready",
}

// riskClause is the clause for a risk finding; an unknown risk falls
// back to its summary as a predicate.
func riskClause(f detection.Finding) string {
	if clause, ok := riskClauses[f.Reason]; ok {
		return clause
	}
	return predicate(f.Entity, f.Summary)
}
