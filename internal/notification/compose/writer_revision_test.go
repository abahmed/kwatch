package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

var revisionNow = time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

func crashIncident() incident.Incident {
	root := inventory.CoreID("deployment", "shop", "web")
	pod := inventory.CoreID("pod", "shop", "web-a")
	s := detection.Finding{
		Entity: pod, Reason: "CrashLoopBackOff",
		Severity: detection.Warning, Since: revisionNow.Add(-time.Hour),
		Summary: "Pod keeps crashing",
	}
	return incident.Incident{
		ID: "inc-20240115-0001", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: revisionNow.Add(-time.Hour),
		Members: map[detection.Key]detection.Finding{s.Key(): s},
	}
}

func TestWriteSupersededResolveMakesNoHealthyClaim(t *testing.T) {
	p := crashIncident()
	p.Members = nil
	p.State, p.Resolved = incident.Resolved, revisionNow
	p.SupersededBy = "inc-20240115-0002"
	d := incident.Decision{
		Action: incident.Resolve, Incident: p,
		Reason: incident.ReasonSuperseded,
	}

	msg := Writer{}.Write(d, revisionNow)

	if msg.Status != notification.StatusResolved {
		t.Fatalf("status = %v, want resolved to close the thread",
			msg.Status)
	}
	if strings.Contains(msg.Note, "healthy again") ||
		msg.Marker == notification.MarkerResolved {
		t.Fatalf("superseded incident must not claim recovery: %s",
			msg.Note)
	}
	if !strings.Contains(msg.Note, "incident inc-20240115-0002") ||
		!strings.Contains(msg.Note, "not resolved") {
		t.Fatalf("message should name the successor: %s", msg.Note)
	}
}

func TestWriteCauseRevisedUpdateSaysSo(t *testing.T) {
	p := crashIncident()
	p.Root = inventory.CoreID("node", "", "n1")
	d := incident.Decision{
		Action: incident.Update, Incident: p,
		Reason: incident.ReasonCauseRevised,
	}

	msg := Writer{}.Write(d, revisionNow)

	if msg.Key != p.ID {
		t.Fatalf("key = %q, want the incident ID %q", msg.Key, p.ID)
	}
	if !strings.HasPrefix(msg.Title, "Node n1 is the revised cause: ") {
		t.Fatalf("lead should state the revision: %q", msg.Title)
	}
	d.Reason = "material change"
	plain := Writer{}.Write(d, revisionNow)
	if strings.Contains(plain.Note, "revised cause") {
		t.Fatalf("ordinary update must not claim a revision: %q",
			plain.Note)
	}
}

func TestWriteNoCauseQuotesOutputOnlyWhenPresent(t *testing.T) {
	d := incident.Decision{Action: incident.Announce,
		Incident: crashIncident()}

	without := Writer{}.Write(d, revisionNow)
	d.Facts.Output = []string{"panic: missing key", ""}
	with := Writer{}.Write(d, revisionNow)

	if !strings.Contains(without.Note, "Nothing outside it explains this.") {
		t.Fatalf("note should say nothing outside explains it: %q",
			without.Note)
	}
	if strings.Contains(without.Note, "last output") {
		t.Fatalf("no output must not be quoted: %s", without.Note)
	}
	if !strings.Contains(with.Note,
		`Its last output was "panic: missing key".`) {
		t.Fatalf("last output line should be quoted: %s", with.Note)
	}
	if len(with.Output) != 2 {
		t.Fatalf("output should stay available to renderers: %v",
			with.Output)
	}
}
