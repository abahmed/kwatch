package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
