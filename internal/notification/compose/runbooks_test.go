package compose

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func runbookDecision(
	action incident.Action, reasons ...string,
) incident.Decision {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	id := inventory.CoreID("pod", "default", "api-1")
	p := incident.Incident{
		ID: "rb", Root: id, Tier: incident.Notify, Opened: now,
		Resolved: now, Members: map[detection.Key]detection.Finding{},
	}
	for _, r := range reasons {
		s := detection.Finding{Entity: id, Reason: r, Since: now,
			Severity: detection.Critical, Summary: r + " happened"}
		p.Members[s.Key()] = s
	}
	return incident.Decision{Action: action, Incident: p}
}

func runbookTexts(w Writer, d incident.Decision) []string {
	msg := w.Write(d, d.Incident.Opened.Add(time.Minute))
	var out []string
	for _, s := range msg.Steps {
		if s.Command == "" {
			out = append(out, s.Text)
		}
	}
	return out
}

func TestWriterRunbookStepsSortedAndCaseInsensitive(t *testing.T) {
	w := NewWriter("", map[string]string{
		"OOMKILLED": "https://rb/oom", "crashloop": "https://rb/crash",
	})
	d := runbookDecision(incident.Announce, "OOMKilled", "CrashLoop", "Other")
	got := runbookTexts(w, d)
	want := []string{
		"Runbook for CrashLoop: https://rb/crash",
		"Runbook for OOMKilled: https://rb/oom",
	}
	if len(got) < 2 || got[len(got)-2] != want[0] ||
		got[len(got)-1] != want[1] {
		t.Fatalf("got %v want suffix %v", got, want)
	}
}

func TestWriterRunbookStepsSkippedOnResolve(t *testing.T) {
	w := NewWriter("", map[string]string{"oomkilled": "https://rb/oom"})
	d := runbookDecision(incident.Resolve, "OOMKilled")
	if got := w.Write(d, d.Incident.Resolved); len(got.Steps) != 0 {
		t.Fatalf("unexpected steps %v", got.Steps)
	}
}

func TestWriterZeroValueAddsNoRunbooks(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	for _, text := range runbookTexts(Writer{}, d) {
		if len(text) > 8 && text[:8] == "Runbook " {
			t.Fatalf("unexpected runbook step %q", text)
		}
	}
}
