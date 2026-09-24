package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestAuditLoggerWritesIncidentAndSkipEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	logger := NewLogger(Config{
		Enabled: true,
		Output:  path,
		Now:     func() time.Time { return now },
	})
	incident := &model.Incident{
		Subject: model.Subject{
			Key:       "pod/default/api",
			ID:        "incident-id",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Name:      "api",
		},
		Status: model.Status{
			Count:     2,
			Severity:  model.SeverityHigh,
			FirstSeen: now.Add(-time.Minute),
			LastSeen:  now,
		},
	}
	logger.LogIncident(incident, model.ActionCreate)
	logger.LogIncident(incident, model.ActionUpdate)
	logger.LogIncident(incident, model.ActionResolved)
	logger.LogIncident(incident, model.ActionSkip)
	logger.LogSkip(incident, "cooldown")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	var entries []Entry
	for _, line := range splitLines(data) {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode audit entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if len(entries) != 5 {
		t.Fatalf("entries = %d, want 5", len(entries))
	}
	if entries[0].Action != ActionCreate || entries[0].Duration != "1m0s" {
		t.Fatalf("first entry = %+v", entries[0])
	}
	if entries[4].Action != ActionSkip || entries[4].SkipReason != "cooldown" {
		t.Fatalf("skip entry = %+v", entries[4])
	}
}

func TestAuditDisabledLoggerAndActionJSON(t *testing.T) {
	logger := NewLogger(Config{Now: time.Now})
	logger.LogIncident(&model.Incident{}, model.ActionCreate)
	logger.LogSkip(&model.Incident{}, "disabled")
	if err := logger.Close(); err != nil {
		t.Fatalf("close disabled logger: %v", err)
	}

	for _, action := range []Action{
		ActionCreate, ActionUpdate, ActionResolved, ActionSkip,
	} {
		data, err := json.Marshal(action)
		if err != nil {
			t.Fatalf("marshal %q: %v", action, err)
		}
		var decoded Action
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal %q: %v", action, err)
		}
		if decoded != action || decoded.String() != string(action) {
			t.Fatalf("decoded action = %q, want %q", decoded, action)
		}
	}
	var action Action
	if err := json.Unmarshal([]byte("3"), &action); err == nil {
		t.Fatal("numeric action unexpectedly decoded")
	}
}

func TestAuditActionMappingDefaultsToSkip(t *testing.T) {
	logger := NewLogger(Config{Now: time.Now})
	if got := logger.actionFromIncidentAction(
		model.IncidentAction(99),
	); got != ActionSkip {
		t.Fatalf("unknown action = %q, want skip", got)
	}
}

func TestAuditLoggerRecordsDiagnosisAndDecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	logger := NewLogger(Config{
		Enabled: true, Output: path, Now: func() time.Time { return now },
	})
	inc := &model.Incident{
		Subject: model.Subject{
			ID: "incident-2", Key: "pod/apps/api", Namespace: "apps",
			Reason: "ContainersNotReady", Name: "api",
		},
		Status: model.Status{
			FirstSeen: now.Add(-5 * time.Minute), LastSeen: now,
			Severity: model.SeverityWarning, PeakResources: 3,
		},
		Delivery: model.Delivery{Revision: 4},
	}
	logger.LogIncidentWithInsight(inc, model.ActionUpdate, &insight.Insight{
		Pattern: "node_failure", Confidence: 0.95,
		CauseState: insight.CauseConfirmed,
		RootCause:  model.ObjectRef{Kind: "node", Name: "worker-a"},
		Evidence:   []string{"node is not ready"},
		Candidates: []insight.CauseCandidate{{
			Ref:   model.ObjectRef{Kind: "node", Name: "worker-a"},
			Score: 120, Supporting: []string{"node is not ready"},
		}},
		Timeline: []insight.TimelineEntry{{
			At: now.Add(-4 * time.Minute), Kind: "failure", Summary: "started",
		}},
		SuppressReason: "expected_rollout_with_capacity",
	})
	require.NoError(t, logger.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry Entry
	require.NoError(t, json.Unmarshal(splitLines(data)[0], &entry))
	require.Equal(t, ActionUpdate, entry.Action)
	require.Equal(t, "node_failure", entry.Pattern)
	require.Equal(t, "confirmed", entry.CauseState)
	require.Equal(t, "node worker-a", entry.RootCause)
	require.Equal(t, "suppress", entry.Decision)
	require.Equal(t, "expected_rollout_with_capacity", entry.DecisionReason)
	require.Len(t, entry.Candidates, 1)
	require.Len(t, entry.Timeline, 1)
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		index := 0
		for index < len(data) && data[index] != '\n' {
			index++
		}
		if index > 0 {
			lines = append(lines, data[:index])
		}
		if index == len(data) {
			break
		}
		data = data[index+1:]
	}
	return lines
}
