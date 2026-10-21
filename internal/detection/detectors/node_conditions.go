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

// problemNodeConditions are the condition types node-problem-detector
// sets when it finds a fault. A True condition of one of them is worth a
// digest line even when the reporter gave no message.
var problemNodeConditions = map[string]bool{
	"KernelDeadlock": true, "ReadonlyFilesystem": true,
	"CorruptDockerOverlay2": true, "FrequentKubeletRestart": true,
	"FrequentDockerRestart": true, "FrequentContainerdRestart": true,
}

// nodeConditionFindings reports every other condition of the node that
// is True: components and controllers that set conditions on a Node
// (the kubelet, the cloud controller, a node problem reporter) use True
// for "this is wrong". The type is evidence, with the message the
// reporter gave; the reason is generic. A condition with no message and
// a type not in problemNodeConditions says nothing a reader can use, so
// it is advisory: evidence inside the incident of a real failure of the
// node, never a digest line of its own.
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
			Evidence: conditionEvidence(e, conditionType),
			Advisory: !readableNodeCondition(e, conditionType),
		})
		if len(out) == maxNodeConditions {
			break
		}
	}
	return out
}

// readableNodeCondition reports a True condition worth a line of its
// own: it carries a message, or its type is a known node problem.
func readableNodeCondition(e inventory.Entity, conditionType string) bool {
	return problemNodeConditions[conditionType] ||
		text(e, kube.ConditionKey(conditionType)+
			kube.AttrConditionMessage) != ""
}

// extraConditionTypes lists, sorted, the condition types of the node
// that no other check reads and that can be bad news when True.
func extraConditionTypes(e inventory.Entity) []string {
	var out []string
	for name := range e.Attributes {
		rest, ok := strings.CutPrefix(name, kube.AttrConditionPrefix)
		if !ok || rest == "" || strings.Contains(rest, ".") ||
			readNodeConditions[rest] || kube.HealthyWhenTrue(rest) {
			continue
		}
		out = append(out, rest)
	}
	sort.Strings(out)
	return out
}

// conditionEvidence names the condition and quotes its message.
func conditionEvidence(
	e inventory.Entity, conditionType string,
) []detection.Evidence {
	evidence := []detection.Evidence{
		{Label: "condition", Value: conditionType + "=True"}}
	message := text(e, kube.ConditionKey(conditionType)+
		kube.AttrConditionMessage)
	if message != "" {
		evidence = append(evidence,
			detection.Evidence{Label: "message", Value: message})
	}
	return evidence
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}
