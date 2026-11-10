package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
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
	if !strings.Contains(text,
		"Cause not found yet: nothing outside it points to one") {
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

	want := "Cause not found yet: its configuration, image and node " +
		"are healthy and unchanged, so look at the app itself."
	if !strings.Contains(text, want) {
		t.Fatalf("note = %s\nwant %q", text, want)
	}
}

// A caused-by line names a likely cause, so the note never says beside
// it that the cause is not clear or that nothing points to one.
func TestCausedByLineSilencesNoCauseSentences(t *testing.T) {
	for name, own := range map[string]bool{
		"unclear": true, "checked": false,
	} {
		p := unclearIncident(own)
		p.CausedBy = []incident.Related{nodeCause}

		text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

		for _, no := range []string{unclearText, "Cause not found yet",
			"Caused by: "} {
			if strings.Contains(text, no) {
				t.Errorf("%s: said %q:\n%s", name, no, text)
			}
		}
		want := "Likely caused by: Node n1 (prod-eu-1) is not ready; " +
			"kwatch is still checking."
		if !strings.Contains(text, want) {
			t.Errorf("%s: want %q:\n%s", name, want, text)
		}
	}
}

// With a cause of its own the caused-by line keeps its plain wording.
func TestCausedByLineIsPlainWithACause(t *testing.T) {
	p := unclearIncident(true)
	p.CausedBy = []incident.Related{nodeCause}
	p.Cause = &rootcause.CauseRecord{
		Root: inventory.CoreID(kube.KindNode, "", "n1"), Score: 0.9,
		RootFindings: []detection.Finding{{Reason: "NodeNotReady",
			Summary: "Node is not ready"}},
	}

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	if !strings.Contains(text, "Caused by: Node n1 (prod-eu-1) is "+
		"not ready.") || strings.Contains(text, "Likely caused") {
		t.Fatalf("want the plain caused-by line:\n%s", text)
	}
}

// The tie to another incident comes from that incident's failure
// chain, which is Pro detail: a Community message never names it, and
// the sentences that say no cause is known speak as without the tie.
func TestCommunityWriterDropsTheCausedByLine(t *testing.T) {
	community := Writer{}.ForEdition(feature.CommunityEntitlements(), true)
	wants := map[string]string{
		"unclear": unclearText, "checked": "Cause not found yet",
	}
	for name, own := range map[string]bool{
		"unclear": true, "checked": false,
	} {
		tied := unclearIncident(own)
		tied.CausedBy = []incident.Related{nodeCause}
		tied.Causing = []incident.Related{nodeCause}

		text := notification.Text(community.Write(announce(tied), at(6, 0)))
		alone := notification.Text(community.Write(
			announce(unclearIncident(own)), at(6, 0)))

		if strings.Contains(text, "aused by") {
			t.Errorf("%s: named the tie:\n%s", name, text)
		}
		if text != alone {
			t.Errorf("%s: tie changed the message:\n%s\nwant\n%s",
				name, text, alone)
		}
		if !strings.Contains(text, wants[name]) {
			t.Errorf("%s: want %q:\n%s", name, wants[name], text)
		}
	}
}
