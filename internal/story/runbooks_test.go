package story

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

func runbookDecision(
	action problem.Action, reasons ...string,
) problem.Decision {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	id := knowledge.NewEntityID("pod", "default", "api-1")
	p := problem.Problem{
		ID: "rb", Root: id, Tier: problem.Notify, Opened: now,
		Resolved: now, Members: map[signal.Key]signal.Signal{},
	}
	for _, r := range reasons {
		s := signal.Signal{Entity: id, Reason: r, Since: now,
			Severity: signal.Critical, Summary: r + " happened"}
		p.Members[s.Key()] = s
	}
	return problem.Decision{Action: action, Problem: p}
}

func runbookTexts(w Writer, d problem.Decision) []string {
	msg := w.Write(d, d.Problem.Opened.Add(time.Minute))
	var out []string
	for _, s := range msg.Steps {
		if s.Command == "" {
			out = append(out, s.Text)
		}
	}
	return out
}

func TestWriterRunbookStepsSortedAndCaseInsensitive(t *testing.T) {
	w := NewWriter(map[string]string{
		"OOMKILLED": "https://rb/oom", "crashloop": "https://rb/crash",
	})
	d := runbookDecision(problem.Announce, "OOMKilled", "CrashLoop", "Other")
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
	w := NewWriter(map[string]string{"oomkilled": "https://rb/oom"})
	d := runbookDecision(problem.Resolve, "OOMKilled")
	if got := w.Write(d, d.Problem.Resolved); len(got.Steps) != 0 {
		t.Fatalf("unexpected steps %v", got.Steps)
	}
}

func TestWriterZeroValueAddsNoRunbooks(t *testing.T) {
	d := runbookDecision(problem.Announce, "OOMKilled")
	for _, text := range runbookTexts(Writer{}, d) {
		if len(text) > 8 && text[:8] == "Runbook " {
			t.Fatalf("unexpected runbook step %q", text)
		}
	}
}
