package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestConditionTypeAndReasonAreBoundedAndRedacted(t *testing.T) {
	attrs := map[string]inventory.Value{}
	long := strings.Repeat("x", 5000)
	setConditions(attrs, []condition{{
		Type: "Ready" + long, Status: "False",
		Reason: "password=hunter2 " + long,
	}})

	for name, value := range attrs {
		assert.LessOrEqual(t, len(name), len(AttrConditionPrefix)+
			maxConditionTypeLength+len(AttrConditionReason)+
			len(AttrConditionMessage), name)
		assert.NotContains(t, value.AsText(), "hunter2")
		assert.LessOrEqual(t, len(value.AsText()),
			maxConditionReasonLength+len("…"))
	}
	assert.Len(t, attrs, 2, "status and reason of one condition")
}

func TestShortConditionsAreUnchanged(t *testing.T) {
	attrs := map[string]inventory.Value{}
	setConditions(attrs, []condition{{
		Type: "Ready", Status: "False", Reason: "MinimumReplicasUnavailable",
	}})
	assert.Equal(t, "False", attrs["condition.Ready"].AsText())
	assert.Equal(t, "MinimumReplicasUnavailable",
		attrs["condition.Ready.reason"].AsText())
}
