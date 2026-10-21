package kube

import "strings"

// ConditionRule says which status of a condition type means trouble. It
// is the one table every reader of conditions shares: the generic and the
// custom resource detectors report a condition by it, the ranking of a
// route's or listener's conditions picks the worse status by it, and the
// model keeps the message of a condition by it.
type ConditionRule struct {
	Type string
	// Bad is the status that means trouble: "False" for a type that is
	// good news when True ("Ready"), "True" for one that is bad news
	// ("Degraded"), "Unknown" when the controller cannot tell.
	Bad string
	// Failing marks a condition that means the object does not do its
	// job. The others only mean it does it less well.
	Failing bool
}

// ConditionRules are condition types with one meaning across operators,
// Crossplane, Flux, Argo CD, cert-manager, Tekton, CRDs and the Gateway
// API. A type may have more than one row: Ready is bad when False, and
// less so when Unknown.
var ConditionRules = []ConditionRule{
	{"Ready", "False", true},
	{"Ready", "Unknown", false},
	{"Available", "False", true},
	{"Established", "False", true},
	{"Accepted", "False", true},
	{"Programmed", "False", true},
	{"Succeeded", "False", true},
	{"Stalled", "True", true},
	{"Failing", "True", true},
	{"Failed", "True", true},
	{"Degraded", "True", false},
	{"Synced", "False", false},
	{"Reconciled", "False", false},
	{"ResolvedRefs", "False", false},
	{"Healthy", "False", false},
	{"Conflicted", "True", false},
	{"PartiallyInvalid", "True", false},
	{"Dangling", "True", false},
	{"OverlappingTLSConfig", "True", false},
}

// healthyTrueSuffixes end the type of a condition that is good news when
// True ("Ready", "ContainersReady", "ScalingActive"): nothing is wrong
// while it holds.
var healthyTrueSuffixes = []string{
	"Ready", "Available", "Healthy", "Initialized", "Scheduled",
	"Progressing", "Complete", "Active",
}

// HealthyWhenTrue reports a condition type that is good news when True,
// by its ending. A node cannot be blamed for such a condition.
func HealthyWhenTrue(conditionType string) bool {
	for _, suffix := range healthyTrueSuffixes {
		if strings.HasSuffix(conditionType, suffix) {
			return true
		}
	}
	return false
}

// ConditionFailing reports whether a condition of the type with the
// status is in its failing state. Unknown always is. A type in
// ConditionRules fails at its bad status, one that is good news when True
// fails when it is not True, and any other type could mean either by its
// name, so every status counts: the message is worth keeping.
func ConditionFailing(conditionType, status string) bool {
	if status == "Unknown" {
		return true
	}
	listed := false
	for _, rule := range ConditionRules {
		if rule.Type != conditionType {
			continue
		}
		listed = true
		if rule.Bad == status {
			return true
		}
	}
	switch {
	case listed:
		return false
	case HealthyWhenTrue(conditionType):
		return status != "True"
	}
	return true
}

// badStatus is the status that is the worst for the type: its first row,
// or "False" for a type that is not listed.
func badStatus(conditionType string) string {
	for _, rule := range ConditionRules {
		if rule.Type == conditionType {
			return rule.Bad
		}
	}
	return "False"
}
