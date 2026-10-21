package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestEventTextKeepsTheNameOfTheObjectTheEventNames(t *testing.T) {
	message := `Error creating: Internal error occurred: failed calling ` +
		`webhook "image-policy.example.com": failed to call webhook: ` +
		`Post "https://image-policy.policy-system.svc:443/validate?` +
		`timeout=10s": x509: certificate has expired or is not yet ` +
		`valid: current time is after the certificate's end date`
	require.Greater(t, len(message), maxMessageLength)

	got := eventText(message)

	assert.Contains(t, got, `webhook "image-policy.example.com"`)
	assert.True(t, strings.HasSuffix(got, "certificate's end date"), got)
}
