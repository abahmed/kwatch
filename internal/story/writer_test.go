package story

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notice"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestWriteResolved(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")

	p := problem.Problem{
		ID:       "test-1",
		Root:     rootID,
		Tier:     problem.Notify,
		State:    problem.Open,
		Opened:   now.Add(-1 * time.Hour),
		Resolved: now,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{
				Entity: rootID, Reason: "CrashLooping",
				Severity: signal.Critical, Since: now.Add(-1 * time.Hour),
				Summary: "Pod is crash looping",
			}.Key(): {
				Entity: rootID, Reason: "CrashLooping",
				Severity: signal.Critical, Since: now.Add(-1 * time.Hour),
				Summary: "Pod is crash looping",
			},
		},
	}
	d := problem.Decision{Action: problem.Resolve, Problem: p}

	msg := Writer{}.Write(d, now)

	if msg.Status != notice.StatusResolved {
		t.Errorf("notice.Status = %v, want %v", msg.Status, notice.StatusResolved)
	}
	if !strings.Contains(msg.Title, "Resolved") {
		t.Errorf("Title missing 'Resolved': %s", msg.Title)
	}
	if !strings.Contains(msg.Title, "lasted 1h") {
		t.Errorf("Title missing duration: %s", msg.Title)
	}
	if len(msg.Lines) == 0 {
		t.Errorf("Lines empty for resolved problem")
	}
}

func TestWriteAnnounceFlapping(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")

	p := problem.Problem{
		ID:        "test-1",
		Root:      rootID,
		Tier:      problem.Notify,
		State:     problem.Flapping,
		Opened:    now.Add(-2 * time.Hour),
		Announced: now,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: rootID, Reason: "CrashLooping"}.Key(): {
				Entity: rootID, Reason: "CrashLooping",
				Severity: signal.Critical,
				Since:    now.Add(-2 * time.Hour),
				Summary:  "Pod is crash looping",
			},
		},
	}
	d := problem.Decision{Action: problem.Announce, Problem: p}

	msg := Writer{}.Write(d, now)

	if msg.Status != notice.StatusFlapping {
		t.Errorf("notice.Status = %v, want %v", msg.Status, notice.StatusFlapping)
	}
	if !strings.Contains(msg.Title, "keeps failing and recovering") {
		t.Errorf("Title missing flapping marker: %s", msg.Title)
	}
}

func TestWriteHeadlineStatesChange(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("deployment", "default", "api")
	configID := knowledge.NewEntityID("configmap", "default", "cfg")

	p := problem.Problem{
		ID:        "test-1",
		Root:      rootID,
		Tier:      problem.Notify,
		State:     problem.Open,
		Opened:    now.Add(-10 * time.Minute),
		Announced: now,
		Cause: &reason.Hypothesis{
			Score:   reason.High,
			Summary: "ConfigMap change",
			Change: &knowledge.Change{
				Entity: configID,
				At:     now.Add(-5 * time.Minute),
				Actor:  "admin",
			},
		},
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: rootID, Reason: "Ready"}.Key(): {
				Entity: rootID, Reason: "Ready",
				Severity: signal.Critical,
				Since:    now.Add(-5 * time.Minute),
				Summary:  "Deployment is not ready",
			},
		},
	}
	d := problem.Decision{Action: problem.Announce, Problem: p}

	msg := Writer{}.Write(d, now)

	if !strings.Contains(msg.Title, "is failing after ConfigMap change") {
		t.Errorf("Title missing change cause: %s", msg.Title)
	}
}

func TestWriteEvidenceDedup(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")

	p := problem.Problem{
		ID:        "test-1",
		Root:      rootID,
		Tier:      problem.Notify,
		State:     problem.Open,
		Opened:    now.Add(-5 * time.Minute),
		Announced: now,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: rootID, Reason: "CrashLooping"}.Key(): {
				Entity: rootID, Reason: "CrashLooping",
				Severity: signal.Critical,
				Since:    now.Add(-5 * time.Minute),
				Summary:  "Pod is crash looping",
				Evidence: []signal.Evidence{
					{Label: "image", Value: "nginx:1.0"},
					{Label: "reason", Value: "OOMKilled"},
					{Label: "reason", Value: "OOMKilled"},
				},
			},
		},
	}
	d := problem.Decision{Action: problem.Announce, Problem: p}

	msg := Writer{}.Write(d, now)

	reasonCount := 0
	for _, line := range msg.Lines {
		if strings.Contains(line, "reason") {
			reasonCount++
		}
	}
	if reasonCount > 1 {
		t.Errorf("evidence not deduped, got %d reason lines", reasonCount)
	}
}

func TestWriteTimelineMerging(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")

	p := problem.Problem{
		ID:        "test-1",
		Root:      rootID,
		Tier:      problem.Notify,
		State:     problem.Open,
		Opened:    now.Add(-20 * time.Minute),
		Announced: now,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: rootID, Reason: "CrashLooping"}.Key(): {
				Entity: rootID, Reason: "CrashLooping",
				Severity: signal.Critical,
				Since:    now.Add(-20 * time.Minute),
				Summary:  "Pod is crash looping",
			},
		},
		Timeline: []problem.Event{
			{At: now.Add(-10 * time.Minute),
				Text: "Pod crashed (api-1)"},
			{At: now.Add(-10 * time.Minute),
				Text: "Pod crashed (api-2)"},
			{At: now.Add(-10 * time.Minute),
				Text: "Pod crashed (api-3)"},
		},
	}
	d := problem.Decision{Action: problem.Announce, Problem: p}

	msg := Writer{}.Write(d, now)

	if len(msg.Timeline) == 0 {
		t.Error("timeline empty")
	}
	found := false
	for _, line := range msg.Timeline {
		if strings.Contains(line, "3×") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("timeline not merged, got %v", msg.Timeline)
	}
}

func TestWriteImpactLine(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")
	svcID := knowledge.NewEntityID("service", "default", "api")

	p := problem.Problem{
		ID:        "test-1",
		Root:      rootID,
		Tier:      problem.Notify,
		State:     problem.Open,
		Opened:    now.Add(-5 * time.Minute),
		Announced: now,
		Impact:    []knowledge.EntityID{svcID},
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: rootID, Reason: "CrashLooping"}.Key(): {
				Entity: rootID, Reason: "CrashLooping",
				Severity: signal.Critical,
				Since:    now.Add(-5 * time.Minute),
				Summary:  "Pod is crash looping",
			},
		},
	}
	d := problem.Decision{Action: problem.Announce, Problem: p}

	msg := Writer{}.Write(d, now)

	found := false
	for _, line := range msg.Lines {
		if strings.Contains(line, "Impact") &&
			strings.Contains(line, "service") &&
			strings.Contains(line, "api") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("impact line not found in %v", msg.Lines)
	}
}

func TestWriteConfidenceLevels(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rootID := knowledge.NewEntityID("pod", "default", "api-1")

	testCases := []struct {
		name     string
		score    float64
		wantConf string
	}{
		{
			name:     "high",
			score:    reason.High,
			wantConf: "high",
		},
		{
			name:     "likely",
			score:    reason.Likely,
			wantConf: "likely",
		},
		{
			name:     "possible",
			score:    0.3,
			wantConf: "possible",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			p := problem.Problem{
				ID:        "test-1",
				Root:      rootID,
				Tier:      problem.Notify,
				State:     problem.Open,
				Opened:    now.Add(-5 * time.Minute),
				Announced: now,
				Cause: &reason.Hypothesis{
					Score:   tt.score,
					Summary: "test cause",
				},
				Members: map[signal.Key]signal.Signal{
					signal.Signal{Entity: rootID, Reason: "CrashLooping"}.Key(): {
						Entity: rootID, Reason: "CrashLooping",
						Severity: signal.Critical,
						Since:    now.Add(-5 * time.Minute),
						Summary:  "Pod is crash looping",
					},
				},
			}
			d := problem.Decision{
				Action:  problem.Announce,
				Problem: p,
			}

			msg := Writer{}.Write(d, now)

			if msg.Confidence != tt.wantConf {
				t.Errorf("Confidence = %s, want %s", msg.Confidence,
					tt.wantConf)
			}
		})
	}
}

func TestWriteRootOwnSignalInHeadline(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	nodeID := knowledge.NewEntityID("node", "", "node-1")

	p := problem.Problem{
		ID:        "test-1",
		Root:      nodeID,
		Tier:      problem.Page,
		State:     problem.Open,
		Opened:    now.Add(-5 * time.Minute),
		Announced: now,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: nodeID, Reason: "LowMemory"}.Key(): {
				Entity: nodeID, Reason: "LowMemory",
				Severity: signal.Critical,
				Since:    now.Add(-5 * time.Minute),
				Summary:  "Node is low on memory",
				Symptom:  false,
			},
		},
	}
	d := problem.Decision{Action: problem.Announce, Problem: p}

	msg := Writer{}.Write(d, now)

	if !strings.Contains(msg.Title, "is low on memory") {
		t.Errorf("Title should include node's own signal: %s",
			msg.Title)
	}
}

func TestStatusEmoji(t *testing.T) {
	tests := []struct {
		status notice.Status
		want   string
	}{
		{notice.StatusCritical, "🔴"},
		{notice.StatusWarning, "🟠"},
		{notice.StatusFlapping, "🔁"},
		{notice.StatusResolved, "✅"},
	}

	for _, tt := range tests {
		if got := tt.status.Emoji(); got != tt.want {
			t.Errorf("Emoji() = %s, want %s", got, tt.want)
		}
	}
}
