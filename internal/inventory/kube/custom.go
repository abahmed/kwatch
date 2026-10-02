package kube

import (
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// failingWhenTrue are condition types whose True status is the failure;
// every other type fails when False. Kept in sync with the custom
// resource detector.
var failingWhenTrue = map[string]bool{
	"Conflicted": true, "Degraded": true, "Failed": true, "Stalled": true,
	"Dangling": true, "OverlappingTLSConfig": true,
}

// statusConditions returns an object's status conditions merged with the
// per-parent and per-listener conditions the Gateway API keeps outside
// status.conditions: HTTPRoute, GRPCRoute, TLSRoute and TCPRoute report
// under status.parents[].conditions, Gateway listeners under
// status.listeners[].conditions. For each type the worst status wins, so
// one rejecting parent or one unprogrammed listener is visible.
func statusConditions(u *unstructured.Unstructured) []condition {
	merged := unstructuredConditions(u)
	nested := append(
		nestedConditions(u, "parents", parentName),
		nestedConditions(u, "listeners", listenerName)...,
	)
	for _, c := range nested {
		merged = mergeCondition(merged, c)
	}
	return merged
}

// nestedConditions reads status.<field>[].conditions, prefixing messages
// with the entry that reported them.
func nestedConditions(
	u *unstructured.Unstructured, field string,
	name func(map[string]any) string,
) []condition {
	entries, _, _ := unstructured.NestedSlice(u.Object, "status", field)
	var out []condition
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		raw, _, _ := unstructured.NestedSlice(m, "conditions")
		for _, c := range parseConditions(raw) {
			if label := name(m); label != "" && c.Message != "" {
				c.Message = label + ": " + c.Message
			}
			out = append(out, c)
		}
	}
	return out
}

func parentName(m map[string]any) string {
	name, _, _ := unstructured.NestedString(m, "parentRef", "name")
	if name == "" {
		return ""
	}
	return "parent " + name
}

func listenerName(m map[string]any) string {
	if name := str(m, "name"); name != "" {
		return "listener " + name
	}
	return ""
}

// mergeCondition adds c, or replaces the condition of the same type when
// c reports a worse status.
func mergeCondition(conditions []condition, c condition) []condition {
	for i, existing := range conditions {
		if existing.Type != c.Type {
			continue
		}
		if conditionRank(c) > conditionRank(existing) {
			conditions[i] = c
		}
		return conditions
	}
	return append(conditions, c)
}

// conditionRank orders statuses from healthy (0) to failing (2).
func conditionRank(c condition) int {
	bad := "False"
	if failingWhenTrue[c.Type] {
		bad = "True"
	}
	switch c.Status {
	case bad:
		return 2
	case "Unknown":
		return 1
	default:
		return 0
	}
}

// parseConditions decodes a list of metav1.Condition-shaped maps.
func parseConditions(raw []any) []condition {
	out := make([]condition, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := condition{
			Type: str(m, "type"), Status: str(m, "status"),
			Reason: str(m, "reason"), Message: str(m, "message"),
		}
		if at, err := time.Parse(time.RFC3339,
			str(m, "lastTransitionTime")); err == nil {
			c.Since = at
		}
		if c.Type != "" && !strings.ContainsAny(c.Type, ".") {
			out = append(out, c)
		}
	}
	return out
}
