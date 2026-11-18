package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestAuditStepsKindRedactionAndBounds(t *testing.T) {
	// Built from pieces so the source holds no token-shaped literal.
	secret := "Bearer " + strings.Repeat("abcd", 3) + "." +
		strings.Repeat("wxyz", 4)
	steps := []notification.Step{
		{Text: "Read it", Command: "kubectl get pod web -n shop"},
		{Text: "Roll back", Command: "kubectl rollout undo " +
			"deployment/web", Mutating: true},
		{Text: "Revert the change in Git"},
		{Text: "Call", Command: "curl -H 'Authorization: " + secret +
			"' http://svc/x"},
		{Text: "Long", Command: "kubectl get " +
			strings.Repeat("x", 500)},
	}
	for range 6 {
		steps = append(steps, notification.Step{Command: "kubectl get ns"})
	}

	got, more := auditSteps(steps)

	require.Len(t, got, audit.MaxSteps)
	assert.Equal(t, 3, more)
	assert.Equal(t, audit.StepReadOnly, got[0].Kind)
	assert.Equal(t, audit.StepMutating, got[1].Kind)
	assert.Equal(t, audit.StepAdvice, got[2].Kind)
	assert.Equal(t, "Revert the change in Git", got[2].Text)
	assert.NotContains(t, got[3].Command, secret)
	assert.NotContains(t, got[3].Command, "abcdabcd")
	assert.LessOrEqual(t, len([]rune(got[4].Command)), audit.MaxStepText)
	assert.True(t, strings.HasSuffix(got[4].Command, "…"))
}

func TestAuditEntryCarriesTheSteps(t *testing.T) {
	d := incident.Decision{Incident: incident.Incident{ID: "inc-1"},
		Action: incident.Announce}
	m := notification.Message{Steps: []notification.Step{
		{Command: "kubectl get pod a"}}}

	entry := AuditEntry(d, m, time.Time{})

	assert.Equal(t, []audit.StepRecord{{Kind: audit.StepReadOnly,
		Command: "kubectl get pod a"}}, entry.Steps)
	assert.Zero(t, entry.StepsMore)
	line, err := json.Marshal(AuditEntry(d, notification.Message{},
		time.Time{}))
	require.NoError(t, err)
	assert.NotContains(t, string(line), `"steps"`)
}
