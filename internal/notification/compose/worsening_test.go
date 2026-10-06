package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func worseningUpdate(crash detection.Finding) string {
	p := badRollout()
	p.Members[crash.Key()] = crash
	p.Revision = 2
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Pod has not been ready for 3m " +
			"(pod shop/payments-7d9f)"},
		{At: at(7, 0), Text: "recovered: Pod has not been ready for 3m " +
			"(pod shop/payments-7d9f)"},
		{At: at(7, 0), Text: crash.Summary + " (container shop/" +
			"payments-7d9f/app)"},
	}
	p.Reported, p.ReportedKnown = 1, true
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}
	return Writer{}.Write(d, at(7, 0)).Note
}

func crashFinding(mode detection.Mode) detection.Finding {
	return detection.Finding{
		Entity: inventory.CoreID(kube.KindContainer, "shop",
			"payments-7d9f/app"),
		Reason: "CrashLoopBackOff", Severity: detection.Critical,
		Since: at(7, 0), Mode: mode,
		Summary: "Container is crash looping",
	}
}

// A pod that goes from not ready to crash-looping is the incident
// getting worse, and the update says so instead of calling the old
// finding a recovery.
func TestUpdateSaysTheIncidentGotWorseWhenACrashLoopBegins(t *testing.T) {
	note := worseningUpdate(crashFinding(detection.ModeCrashLoop))
	if !strings.Contains(note, "is getting worse") ||
		!strings.Contains(note, "now crash-loops") {
		t.Fatalf("update must say the failure got worse: %s", note)
	}
	if strings.Contains(note, "has recovered") {
		t.Fatalf("a crash loop is not a recovery: %s", note)
	}
}

// Without a crash loop among the new members the update reads as before.
func TestUpdateWithoutACrashLoopIsNotWorse(t *testing.T) {
	note := worseningUpdate(crashFinding(detection.ModeNotReady))
	if strings.Contains(note, "is getting worse") {
		t.Fatalf("nothing began to crash-loop: %s", note)
	}
}
