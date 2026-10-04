package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightRisk ranks a risk's cost below usage facts and above a quoted
// error: it explains why the failure hurt, not what failed.
const weightRisk = 0.5

// maxRiskSentences bounds how many risks one note names.
const maxRiskSentences = 2

// riskSentences say what a configuration risk cost, when the failure in
// front of the reader shows it: a crash loop behind a workload with no
// readiness probe kept receiving traffic. A risk the failure does not
// touch stays in the digest where the risk detector reported it.
func riskSentences(f caseFacts) []sentence {
	if onlyAdvisory(f.members) {
		return nil
	}
	var out []sentence
	for _, m := range f.members {
		if !m.Advisory || m.Entity != f.p.Root ||
			len(out) == maxRiskSentences {
			continue
		}
		if text := riskCost(f, m.Reason); text != "" {
			out = append(out, sentence{part: partConsequence,
				weight: weightRisk, text: text})
		}
	}
	return out
}

// riskCost words one risk when the incident shows its cost; "" when it
// does not.
func riskCost(f caseFacts, risk string) string {
	switch risk {
	case reasons.RiskNoReadinessProbe:
		// A crashed container is unready anyway; the probe matters for
		// a running one that cannot serve.
		if anyMemberMode(f, detection.ModeProbe, detection.ModeActiveProbe) {
			return "It has no readiness probe, so traffic kept reaching " +
				"it while it failed."
		}
	case reasons.RiskNoMemoryLimit:
		if anyMemberMode(f, detection.ModeOOMKilled, detection.ModeEvicted) ||
			causeRuleIs(f, "node-memory-pressure", "node-overcommitted") {
			return "Its containers have no memory limit, so nothing " +
				"stopped them before the node ran short."
		}
	case reasons.RiskSingleReplica:
		if anyMemberMode(f, detection.ModeCrashLoop, detection.ModeNotReady,
			detection.ModeUnavailable, detection.ModeNoEndpoints) {
			return "It runs a single replica, so this is downtime, not " +
				"degradation."
		}
	case reasons.RiskMutableImageTag:
		if f.p.Cause == nil && anyMemberMode(f, detection.ModeCrashLoop) {
			return "Its image tag is not fixed, so what runs may have " +
				"changed without a rollout."
		}
	}
	return ""
}

// anyMemberMode reports whether a failing member shows one of modes, or
// a finer mode below it.
func anyMemberMode(f caseFacts, modes ...detection.Mode) bool {
	for _, m := range f.members {
		if m.Advisory {
			continue
		}
		for _, mode := range modes {
			if m.Mode == mode || strings.HasPrefix(string(m.Mode),
				string(mode)+".") {
				return true
			}
		}
	}
	return false
}

// causeRuleIs reports whether the incident's cause came from one of the
// named rows.
func causeRuleIs(f caseFacts, rules ...string) bool {
	if f.p.Cause == nil {
		return false
	}
	for _, rule := range rules {
		if f.p.Cause.Rule == rule {
			return true
		}
	}
	return false
}
