package message

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			Impact: "3 replicas are unavailable", NextSteps: []string{"inspect"},
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
	require.NotNil(t, notification.Summary.PrimaryAction)
	assert.Equal(
		t,
		[]string{"kubectl", "describe", "deployment", "checkout", "-n", "payments"},
		notification.Summary.PrimaryAction.Args,
	)
	assert.Len(t, notification.Details, 8)
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
	require.NotNil(t, notification.Summary.PrimaryAction)
	assert.Equal(t, "get", notification.Summary.PrimaryAction.Args[1])
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
