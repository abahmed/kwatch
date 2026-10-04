package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const unclearText = "The cause is not clear yet."

func unclearIncident(ownFinding bool) incident.Incident {
	reports := inventory.CoreID(kube.KindDeployment, "shop", "reports")
	ct := inventory.CoreID(kube.KindContainer, "shop", "reports-8b1/app")
	p := incident.Incident{
		ID: "inc-5", Root: reports, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		CauseUnclear: true,
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(0, 30), Summary: "Container is crash looping"}),
	}
	if ownFinding {
		stuck := detection.Finding{Entity: reports,
			Reason:   reasons.ProgressDeadlineExceeded,
			Severity: detection.Critical, Since: at(0, 0),
			Summary: "Rollout is stuck"}
		p.Members[stuck.Key()] = stuck
	}
	return p
}

func TestUnclearCauseIsSaidOnceAfterTheLead(t *testing.T) {
	msg := Writer{}.Write(announce(unclearIncident(true)), at(6, 0))

	text := notification.Text(msg)
	if strings.Count(text, unclearText) != 1 {
		t.Fatalf("want the unclear sentence once:\n%s", text)
	}
	if len(msg.Lines) == 0 || msg.Lines[0] != unclearText {
		t.Fatalf("the sentence follows the lead: %q", msg.Lines)
	}
}

// Without the root's own finding the lead already says no outside
// cause was found; a hedged cause is still a cause.
func TestUnclearCauseIsNotRepeatedOrContradicted(t *testing.T) {
	noRootFinding := unclearIncident(false)
	withCause := unclearIncident(true)
	withCause.Cause = &rootcause.CauseRecord{
		Root: inventory.CoreID(kube.KindSecret, "shop", "db"), Score: 0.4,
		RootFindings: []detection.Finding{{Reason: "Missing",
			Summary: "Secret does not exist"}},
	}
	notFlagged := unclearIncident(true)
	notFlagged.CauseUnclear = false

	for name, p := range map[string]incident.Incident{
		"no root finding": noRootFinding, "with cause": withCause,
		"not flagged": notFlagged,
	} {
		text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))
		if strings.Contains(text, unclearText) {
			t.Errorf("%s: said the cause is unclear:\n%s", name, text)
		}
	}
	text := notification.Text(Writer{}.Write(announce(noRootFinding),
		at(6, 0)))
	if !strings.Contains(text, "Nothing outside it explains this.") {
		t.Errorf("the note must still say nothing outside explains it:\n%s",
			text)
	}
}

// Without a cause, the note says what was checked and found healthy
// instead of saying no cause was found.
func TestCheckedSentencesNameWhatWasHealthy(t *testing.T) {
	p := unclearIncident(false)
	p.CauseUnclear = false
	p.Checked = []string{"configmap", "image", "node", "secret", "zone"}

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	want := "Its configuration, image and node are healthy and " +
		"unchanged, so nothing outside it explains this."
	if !strings.Contains(text, want) {
		t.Fatalf("note = %s\nwant %q", text, want)
	}
}
