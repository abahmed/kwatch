package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestConditionFailingFollowsTheStatusTable(t *testing.T) {
	cases := []struct {
		name, conditionType, status string
		want                        bool
	}{
		{"ready_true_is_healthy", "Ready", "True", false},
		{"ready_false_fails", "Ready", "False", true},
		{"ready_unknown_fails", "Ready", "Unknown", true},
		{"degraded_true_fails", "Degraded", "True", true},
		{"degraded_false_is_healthy", "Degraded", "False", false},
		{"failing_true_fails", "Failing", "True", true},
		{"succeeded_false_fails", "Succeeded", "False", true},
		{"succeeded_true_is_healthy", "Succeeded", "True", false},
		{"unlisted_true_fails", "KernelDeadlock", "True", true},
		{"unlisted_false_keeps", "KernelDeadlock", "False", true},
		{"healthy_suffix_true", "ContainerRuntimeReady", "True", false},
		{"healthy_suffix_false", "ContainerRuntimeReady", "False", true},
		{"progressing_true", "Progressing", "True", false},
		{"progressing_false", "Progressing", "False", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want,
				ConditionFailing(tc.conditionType, tc.status))
		})
	}
}

func TestSetConditionsKeepsTheMessageOfEveryFailingCondition(t *testing.T) {
	failing := []condition{
		{Type: "Degraded", Status: "True", Message: "2 of 3 pods crash"},
		{Type: "Failing", Status: "True", Message: "operator is failing"},
		{Type: "Succeeded", Status: "False", Message: "step build failed"},
		{Type: "Ready", Status: "Unknown", Message: "waiting"},
		{Type: "KernelDeadlock", Status: "True", Message: "docker hung"},
	}
	healthy := []condition{
		{Type: "Ready", Status: "True", Message: "all good"},
		{Type: "Degraded", Status: "False", Message: "not degraded"},
		{Type: "Succeeded", Status: "True", Message: "done"},
	}
	attrs := map[string]inventory.Value{}
	setConditions(attrs, failing)
	for _, c := range failing {
		assert.Equal(t, c.Message, attrs[ConditionKey(c.Type)+
			AttrConditionMessage].AsText(), c.Type)
	}
	for _, c := range healthy {
		attrs = map[string]inventory.Value{}
		setConditions(attrs, []condition{c})
		_, kept := attrs[ConditionKey(c.Type)+AttrConditionMessage]
		assert.False(t, kept, "%s=%s is healthy", c.Type, c.Status)
	}
}

func TestMergeConditionRanksByTheStatusTable(t *testing.T) {
	merged := mergeCondition(
		[]condition{{Type: "Failing", Status: "False"}},
		condition{Type: "Failing", Status: "True", Message: "worse"})
	assert.Equal(t, "True", merged[0].Status,
		"Failing is bad when True, like Degraded")
	merged = mergeCondition(
		[]condition{{Type: "Succeeded", Status: "True"}},
		condition{Type: "Succeeded", Status: "False", Message: "worse"})
	assert.Equal(t, "False", merged[0].Status)
}
