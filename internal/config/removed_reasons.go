package config

import (
	"fmt"
	"strings"
)

// removedReasons are the reasons a release removed because nothing
// raised them. A rule that names one still loads, but matches nothing.
var removedReasons = []string{
	"ContainerCannotRun", "Killed", "DeploymentProgressingFalse",
	"DeploymentAvailableFalse", "StatefulSetConditionFailure",
	"DaemonSetConditionFailure", "ResourceHigh", "ResourceCritical",
}

// removedReason is the canonical name of reason when it was removed;
// reasons match without regard to case, and a reasons entry may start
// with "!".
func removedReason(reason string) (string, bool) {
	name := strings.TrimSpace(strings.TrimPrefix(reason, "!"))
	for _, removed := range removedReasons {
		if strings.EqualFold(name, removed) {
			return removed, true
		}
	}
	return "", false
}

// removedReasonWarnings reports every place that names a removed
// reason: reasons, severityByReason, templates, runbooks and the
// silences. Each reason is reported once per place, in a stable order.
func removedReasonWarnings(cfg *Config) []string {
	var out []string
	add := func(place string, names []string) {
		seen := map[string]bool{}
		for _, name := range names {
			removed, ok := removedReason(name)
			if !ok || seen[removed] {
				continue
			}
			seen[removed] = true
			out = append(out, fmt.Sprintf("reason `%s` in %s was "+
				"removed in this release; this rule matches nothing",
				removed, place))
		}
	}
	add("reasons", cfg.Reasons)
	add("severityByReason", sortedStringKeys(cfg.SeverityByReason))
	add("templates", sortedStringKeys(cfg.Templates))
	add("runbooks", sortedStringKeys(cfg.Runbooks))
	for i, rule := range userSilences(cfg) {
		add(fmt.Sprintf("silences[%d].reasons", i), rule.Reasons)
	}
	return out
}

// RemovedReasonsError fails `kwatch lint --strict` when a rule names a
// removed reason; it is nil otherwise.
func RemovedReasonsError(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	warnings := removedReasonWarnings(cfg)
	if len(warnings) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(warnings, "; "))
}
