package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func conditionMap(t, status, reason, message string) map[string]any {
	return map[string]any{
		"type": t, "status": status, "reason": reason, "message": message,
		"lastTransitionTime": "2024-01-01T12:00:00Z",
	}
}

func byType(conditions []condition) map[string]condition {
	out := make(map[string]condition, len(conditions))
	for _, c := range conditions {
		out[c.Type] = c
	}
	return out
}

func TestStatusConditionsReadRouteParents(t *testing.T) {
	route := &unstructured.Unstructured{Object: map[string]any{
		"kind": "HTTPRoute",
		"status": map[string]any{"parents": []any{
			map[string]any{
				"parentRef": map[string]any{"name": "public"},
				"conditions": []any{
					conditionMap("Accepted", "True", "Accepted", ""),
					conditionMap("ResolvedRefs", "True", "ResolvedRefs",
						""),
				},
			},
			map[string]any{
				"parentRef": map[string]any{"name": "internal"},
				"conditions": []any{
					conditionMap("Accepted", "False", "NotAllowedByListeners",
						"no listener allows this route"),
				},
			},
		}},
	}}

	got := byType(statusConditions(route))

	assert.Equal(t, "False", got["Accepted"].Status,
		"one rejecting parent makes the route not accepted")
	assert.Equal(t, "NotAllowedByListeners", got["Accepted"].Reason)
	assert.Equal(t, "parent internal: no listener allows this route",
		got["Accepted"].Message)
	assert.Equal(t, "True", got["ResolvedRefs"].Status)
	assert.False(t, got["Accepted"].Since.IsZero())
}

func TestStatusConditionsReadGatewayListeners(t *testing.T) {
	gateway := &unstructured.Unstructured{Object: map[string]any{
		"kind": "Gateway",
		"status": map[string]any{
			"conditions": []any{
				conditionMap("Accepted", "True", "Accepted", ""),
				conditionMap("Programmed", "True", "Programmed", ""),
			},
			"listeners": []any{map[string]any{
				"name": "https",
				"conditions": []any{
					conditionMap("Programmed", "False", "Invalid",
						"certificate not found"),
					conditionMap("Conflicted", "True", "HostnameConflict",
						"hostname in use"),
					conditionMap("Accepted", "Unknown", "Pending", ""),
				},
			}},
		},
	}}

	got := byType(statusConditions(gateway))

	assert.Equal(t, "False", got["Programmed"].Status)
	assert.Equal(t, "listener https: certificate not found",
		got["Programmed"].Message)
	assert.Equal(t, "True", got["Conflicted"].Status)
	assert.Equal(t, "Unknown", got["Accepted"].Status,
		"unknown is worse than a healthy top-level condition")
}

func TestStatusConditionsHealthyNestedDoNotHideFailure(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"conditions": []any{
				conditionMap("Accepted", "False", "Invalid", "bad spec"),
			},
			"parents": []any{map[string]any{
				"conditions": []any{
					conditionMap("Accepted", "True", "Accepted", ""),
					"not a map",
				},
			}, "not a map"},
		},
	}}

	got := byType(statusConditions(obj))

	assert.Equal(t, "False", got["Accepted"].Status)
	assert.Equal(t, "bad spec", got["Accepted"].Message)
}

func TestStatusConditionsOverlappingTLSConfigFailsWhenTrue(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"conditions": []any{
				conditionMap("OverlappingTLSConfig", "False", "", ""),
			},
			"parents": []any{map[string]any{
				"conditions": []any{conditionMap(
					"OverlappingTLSConfig", "True", "Overlap", "shared SNI"),
				},
			}},
		},
	}}

	got := byType(statusConditions(obj))

	assert.Equal(t, "True", got["OverlappingTLSConfig"].Status)
	assert.Equal(t, "shared SNI", got["OverlappingTLSConfig"].Message)
}
