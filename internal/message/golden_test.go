package message

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestProductionRenderingGoldens(t *testing.T) {
	report := productionReport()
	incident := &model.Incident{
		Subject:  model.Subject{ID: "incident-1"},
		Delivery: model.Delivery{Revision: 2},
	}
	notification := NotificationFromReport(
		report, incident, "node_failure", 0.9, 1,
	)
	raw, err := json.Marshal(notification)
	require.NoError(t, err)
	assertGolden(
		t, "plain-create.golden", NewPlainTextRenderer().RenderCreate(report),
	)
	assertGolden(t, "slack-create.golden", NewSlackRenderer().RenderCreate(report))
	update := productionReport()
	update.Action = "update"
	update.Summary.Emoji = "🔄"
	update.Summary.Label = "Pod is still restarting"
	update.Summary.Duration = "6m"
	assertGolden(t, "plain-update.golden",
		NewPlainTextRenderer().RenderUpdate(update))
	assertGolden(t, "slack-update.golden",
		NewSlackRenderer().RenderUpdate(update))
	reopen := productionReport()
	reopen.Action = "update"
	reopen.Summary.Emoji = "🔄"
	reopen.Summary.Label = "Pod restarted again"
	reopen.Summary.Duration = "6m"
	assertGolden(t, "plain-reopen.golden",
		NewPlainTextRenderer().RenderUpdate(reopen))
	assertGolden(t, "plain-resolved.golden", NewPlainTextRenderer().RenderResolved(
		productionResolvedReport(),
	))
	assertGolden(t, "webhook-create.json.golden", string(raw))
}

func productionReport() *Report {
	return &Report{
		Action: "create", Severity: "high", Reason: "CrashLoopBackOff",
		Cluster: "production", Namespace: "payments", Resource: "pod",
		Name: "checkout", Summary: SummarySection{
			Emoji: "🟠", Label: "Pod keeps restarting", Duration: "3m",
		},
		Identity: &IdentitySection{
			Container: "api", Image: "registry/checkout:v4", Node: "worker-a",
		},
		Diagnosis: &DiagnosisSection{
			Cause: "the node is not ready", Pattern: "node_failure",
			Confidence: 0.9, CauseState: insight.CauseConfirmed,
			Evidence: []string{"NodeNotReady"},
			Impact:   "3 replicas are unavailable",
		},
		Evidence: &EvidenceSection{
			Events: "NodeNotReady", Logs: "connection refused",
		},
	}
}

func productionResolvedReport() *Report {
	report := productionReport()
	report.Action = "resolved"
	report.Resolution = &model.Resolution{
		Summary:  "the deployment recovered",
		Evidence: "all replicas are ready",
	}
	return report
}

func assertGolden(t *testing.T, name, actual string) {
	t.Helper()
	path := filepath.Join("testdata", "production", name)
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	if strings.TrimSpace(actual) != strings.TrimSpace(string(expected)) {
		t.Fatalf("%s mismatch:\n%s", name, diffText(string(expected), actual))
	}
}

func diffText(expected, actual string) string {
	return "expected:\n" + expected + "\nactual:\n" + actual
}
