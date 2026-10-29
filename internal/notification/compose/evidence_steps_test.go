package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var evidenceAt = time.Date(2026, 10, 29, 13, 44, 0, 0, time.UTC)

// crashDecision is a crash loop whose logs and pod state were read.
func crashDecision(report []incident.Section) incident.Decision {
	id := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	f := detection.Finding{Entity: id, Reason: reasons.CrashLoopBackOff,
		Mode: "CrashLoop", Severity: detection.Critical,
		Since: evidenceAt.Add(-5 * time.Minute)}
	p := incident.Incident{ID: "inc-1", Root: id, State: incident.Open,
		Opened: f.Since, Revision: 1,
		Members: map[detection.Key]detection.Finding{f.Key(): f},
		Cause: &rootcause.CauseRecord{Root: id, Score: rootcause.High,
			Summary: "api crashes"}}
	return incident.Decision{Action: incident.Announce, Incident: p,
		Facts: incident.Facts{Report: report,
			Correlation: &incident.Correlation{
				Confirmed: "the last run exited with code 1"}}}
}

var gatheredReport = []incident.Section{
	{Check: incident.CheckLogs, Status: incident.SectionOK},
	{Check: incident.CheckPodState, Status: incident.SectionOK},
}

func commandsOf(m notification.Message) string {
	var out []string
	for _, s := range m.Steps {
		out = append(out, s.Command)
	}
	return strings.Join(out, "\n")
}

func TestWriteProDropsReadOnlyStepsKwatchRan(t *testing.T) {
	m := NewWriter("", nil).Write(crashDecision(gatheredReport), evidenceAt)
	if strings.Contains(commandsOf(m), "kubectl logs") ||
		strings.Contains(m.Note, "kubectl logs") {
		t.Fatalf("a gathered read is still a step:\n%s\n%s",
			commandsOf(m), m.Note)
	}
	if m.Confidence != confirmedWord ||
		!strings.Contains(m.Note, "Confirmed.") {
		t.Fatalf("confidence %q, note %q", m.Confidence, m.Note)
	}
}

func TestWriteProKeepsStepsWhoseReadFailed(t *testing.T) {
	denied := []incident.Section{{Check: incident.CheckLogs,
		Status: incident.SectionDenied, Reason: incident.ReasonRBAC}}
	m := NewWriter("", nil).Write(crashDecision(denied), evidenceAt)
	if !strings.Contains(commandsOf(m), "kubectl logs") {
		t.Fatalf("a denied read lost its step:\n%s", commandsOf(m))
	}
}

func TestWriteCommunityIgnoresTheReport(t *testing.T) {
	w := NewWriter("", nil).ForEdition(feature.OrCommunity(nil), false)
	with := w.Write(crashDecision(gatheredReport), evidenceAt)
	without := crashDecision(nil)
	without.Facts.Correlation = nil
	plain := w.Write(without, evidenceAt)
	if with.Note != plain.Note || commandsOf(with) != commandsOf(plain) ||
		with.Confidence != plain.Confidence {
		t.Fatalf("Community changed with a report:\n%s\n%s",
			with.Note, plain.Note)
	}
}

// A read that found nothing was still taken: running the command would
// show the reader nothing more, so the step goes.
func TestWriteProDropsStepsWhoseReadFoundNothing(t *testing.T) {
	absent := []incident.Section{{Check: incident.CheckLogs,
		Status: incident.SectionAbsent}}
	m := NewWriter("", nil).Write(crashDecision(absent), evidenceAt)
	if strings.Contains(commandsOf(m), "kubectl logs") {
		t.Fatalf("an absent read kept its step:\n%s", commandsOf(m))
	}
	failed := []incident.Section{{Check: incident.CheckLogs,
		Status: incident.SectionError}}
	m = NewWriter("", nil).Write(crashDecision(failed), evidenceAt)
	if !strings.Contains(commandsOf(m), "kubectl logs") {
		t.Fatalf("a failed read lost its step:\n%s", commandsOf(m))
	}
}
