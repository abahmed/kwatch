package message

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNotificationFromReportBuildsSummaryAndDetails(t *testing.T) {
	report := &Report{
		Action:    "create",
		Severity:  "critical",
		Cluster:   "production",
		Namespace: "payments",
		Resource:  "deployment",
		Name:      "checkout",
		Summary: SummarySection{
			Emoji: "🔴", Label: "Checkout is unavailable",
			Duration: "for 3m",
		},
		Identity: &IdentitySection{
			Container: "api", Image: "registry/checkout:v4", Node: "worker-a",
		},
		Diagnosis: &DiagnosisSection{
			Cause: "the node is not ready", Pattern: "node_failure",
			Impact:     "3 replicas are unavailable",
			CauseState: insight.CauseConfirmed, Confidence: 0.9,
			Evidence: []string{"NodeNotReady"},
		},
		Evidence: &EvidenceSection{
			Events: "NodeNotReady", Logs: "connection refused",
		},
		OOM:     &OOMSection{MemoryLimit: "256Mi", Timeline: "10:00"},
		Probe:   &ProbeSection{ProbeType: "readiness", Endpoint: "GET /ready"},
		Pending: &PendingSection{ResourceRequests: []string{"cpu=2"}},
		Runbook: "https://runbook.example/checkout",
		Resolution: &model.Resolution{
			Summary: "the deployment recovered", Evidence: "all replicas are ready",
		},
	}
	incident := &model.Incident{
		Subject:  model.Subject{ID: "incident-1"},
		Delivery: model.Delivery{Revision: 2},
	}

	notification := NotificationFromReport(
		report, incident, "node_failure", 0.4, 2,
	)
	require.NotNil(t, notification)
	assert.Equal(t, "incident-1:2:create", notification.DeliveryID)
	assert.Equal(t, model.ActionCreate, notification.Action)
	assert.Equal(t, "the node is not ready", notification.Summary.Cause)
	assert.Equal(t, "3 replicas are unavailable", notification.Summary.Impact)
	assert.Contains(t, notification.Summary.Story,
		"🔎 Cause: the node is not ready.")
	assert.Contains(t, notification.Summary.Story, "memory limit is 256Mi")
	assert.Contains(t, notification.Summary.Story, "readiness probe")
	assert.Contains(t, notification.Summary.Story, "Requested resources: cpu=2")
	assert.Nil(t, notification.Summary.PrimaryAction)
	assert.Len(t, notification.Details, 3)
	assert.Equal(t, "🧾 Evidence", notification.Details[0].Title)
	assert.Equal(t, "Logs", notification.Details[1].Title)
	assert.Equal(t, "Runbook", notification.Details[2].Title)
}

func TestNotificationOmitsIrrelevantDetails(t *testing.T) {
	report := &Report{
		Action: "create", Namespace: "payments", Resource: "service",
		Name: "checkout", Summary: SummarySection{Label: "No endpoints"},
	}
	incident := &model.Incident{Subject: model.Subject{ID: "incident-3"}}

	notification := NotificationFromReport(report, incident, "", 0, 0)
	require.NotNil(t, notification)
	assert.Empty(t, notification.Summary.Story)
	assert.Empty(t, notification.Details)
}

func TestNotificationRecoveryOmitsBoilerplateKeepsEvidence(t *testing.T) {
	report := &Report{
		Action:  "resolved",
		Summary: SummarySection{Duration: "5m"},
		Resolution: &model.Resolution{
			Summary:  "the condition recovered",
			Evidence: "the replacement pod is ready",
		},
	}
	incident := &model.Incident{Subject: model.Subject{ID: "incident-4"}}

	notification := NotificationFromReport(report, incident, "", 0, 0)
	require.NotNil(t, notification)
	assert.Contains(t, notification.Summary.Story, "Recovered after 5m.")
	assert.Contains(t, notification.Summary.Story, "The replacement pod is ready.")
	assert.NotContains(t, notification.Summary.Story, "condition recovered")

	for _, renderer := range []Renderer{
		NewPlainTextRenderer(), NewSlackRenderer(), NewDiscordRenderer(),
	} {
		text := renderer.RenderResolved(report)
		assert.Contains(t, text, "replacement pod is ready")
		assert.NotContains(t, text, "condition recovered")
	}
}

func TestTextRendererDoesNotDuplicateNamespace(t *testing.T) {
	report := &Report{
		Action: "create", Namespace: "payments", Name: "payments/checkout",
		Summary: SummarySection{Emoji: "🔴", Label: "Checkout failed"},
	}

	text := NewPlainTextRenderer().RenderCreate(report)
	assert.Contains(t, text, "payments/checkout")
	assert.NotContains(t, text, "payments/payments/checkout")
}

func TestNotificationOmitsWeakCauseAndUnsafeGroupCommand(t *testing.T) {
	report := &Report{
		Action: "unknown", Resource: "deployment",
		Name: "7 workloads in payments", Namespace: "payments",
		Diagnosis: &DiagnosisSection{
			Cause: "a recent change may be related", Pattern: "recent_change",
			Confidence: 0.4, NextSteps: []string{"inspect"},
		},
	}
	incident := &model.Incident{
		Subject: model.Subject{ID: "incident-2"},
	}

	notification := NotificationFromReport(report, incident, "", 0.4, 0)
	require.NotNil(t, notification)
	assert.Equal(t, model.ActionSkip, notification.Action)
	assert.Empty(t, notification.Summary.Cause)
	assert.Nil(t, notification.Summary.PrimaryAction)
}

func TestNotificationFromReportRejectsNilInputs(t *testing.T) {
	incident := &model.Incident{}
	require.Nil(t, NotificationFromReport(nil, incident, "", 0, 0))
	require.Nil(t, NotificationFromReport(&Report{}, nil, "", 0, 0))
	assert.Equal(t, model.ActionSkip, parseAction("not-an-action"))
}

func TestNotificationJSONUsesStableProviderContract(t *testing.T) {
	notification := &Notification{
		DeliveryID: "incident-1:2:create",
		Revision:   2,
		Action:     model.ActionCreate,
		Summary: NotificationSummary{
			Title: "Checkout is unavailable",
			Cause: "the node is not ready",
		},
		Diagnostic: DiagnosticMetadata{
			Pattern: "node_failure", Confidence: 0.99, EvidenceCount: 2,
		},
	}
	raw, err := json.Marshal(notification)
	require.NoError(t, err)
	text := string(raw)
	assert.Contains(t, text, `"deliveryId":"incident-1:2:create"`)
	assert.Contains(t, text, `"action":"create"`)
	assert.NotContains(t, text, "confidence")
	assert.NotContains(t, text, "node_failure")
}
