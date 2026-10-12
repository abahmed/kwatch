package detectors

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxNodeConditions bounds the extra conditions reported for one node.
const maxNodeConditions = 3

// readNodeConditions are the node condition types read elsewhere.
var readNodeConditions = map[string]bool{
	"Ready": true, "MemoryPressure": true, "DiskPressure": true,
	"PIDPressure": true, "NetworkUnavailable": true,
}

// healthyTrueSuffixes end the type of a condition that is good news when
// True ("Ready", "Available"): a node cannot be blamed for it.
var healthyTrueSuffixes = []string{"Ready", "Available", "Healthy"}

// nodeConditionFindings reports every other condition of the node that
// is True: components and controllers that set conditions on a Node
// (the kubelet, the cloud controller, a node problem reporter) use True
// for "this is wrong". The type is evidence; the reason is generic.
func nodeConditionFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	for _, conditionType := range extraConditionTypes(e) {
		status, reason, since := condition(e, conditionType)
		key := "node-condition/" + conditionType
		if since.IsZero() {
			since = ctx.Onset("sustained/"+key, ctx.Now)
		}
		if status != "True" || !sustained(ctx, key, since,
			DefaultNodePressure) {
			continue
		}
		out = append(out, detection.Finding{
			Reason:   reasons.NodeConditionPrefix + conditionType,
			Severity: detection.Warning,
			Mode:     detection.NodeConditionModeOf(conditionType),
			Since:    since,
			Summary: "Node condition " + conditionType + " is True" +
				reasonSuffix(reason),
			Evidence: []detection.Evidence{
				{Label: "condition", Value: conditionType + "=True"}},
		})
		if len(out) == maxNodeConditions {
			break
		}
	}
	return out
}

// extraConditionTypes lists, sorted, the condition types of the node
// that no other check reads and that can be bad news when True.
func extraConditionTypes(e inventory.Entity) []string {
	var out []string
	for name := range e.Attributes {
		rest, ok := strings.CutPrefix(name, kube.AttrConditionPrefix)
		if !ok || rest == "" || strings.Contains(rest, ".") ||
			readNodeConditions[rest] || healthyWhenTrue(rest) {
			continue
		}
		out = append(out, rest)
	}
	sort.Strings(out)
	return out
}

func healthyWhenTrue(conditionType string) bool {
	for _, suffix := range healthyTrueSuffixes {
		if strings.HasSuffix(conditionType, suffix) {
			return true
		}
	}
	return false
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}
