package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestReminderSentencesSayHowLongItHasBeenOpen(t *testing.T) {
	p := badRollout()
	f := caseFacts{p: p, now: p.Opened.Add(14 * 24 * time.Hour)}

	got := reminderSentences(f)

	if len(got) == 0 || got[0].part != partLead {
		t.Fatalf("want a lead sentence first, got %+v", got)
	}
	for _, want := range []string{"is still down", "two weeks now."} {
		if !strings.Contains(got[0].text, want) {
			t.Fatalf("lead %q lacks %q", got[0].text, want)
		}
	}
}

func TestRecurrenceSentenceStatesTheRhythm(t *testing.T) {
	now := goldenStart.Add(2 * time.Hour)
	var times []time.Time
	for i := 2; i >= 0; i-- {
		times = append(times, now.Add(-time.Duration(i)*40*time.Minute))
	}
	f := caseFacts{now: now, p: incident.Incident{Occurrences: times}}

	got := recurrenceSentences(f)

	want := "It fails every 40 minutes or so; this is the third time " +
		"in a day."
	if len(got) != 1 || got[0].text != want {
		t.Fatalf("got %+v, want %q", got, want)
	}
}

func TestResolveNamesTheCauseInPastTense(t *testing.T) {
	p := badRollout()
	p.State, p.Resolved = incident.Resolved, at(42, 0)
	p.Opened = at(0, 0)
	node := inventory.CoreID(kube.KindNode, "", "n3")
	p.Cause = &rootcause.CauseRecord{Root: node, Rule: "node",
		RootFindings: []detection.Finding{{Entity: node,
			Reason: reasons.MemoryPressure, Severity: detection.Critical,
			Summary: "Node is low on memory; pods may be evicted"}},
		Summary: "node n3: Node is low on memory"}
	f := caseFacts{p: p, now: at(43, 0)}

	_, got := resolveNote(f)

	text := got[len(got)-1].text
	if !strings.Contains(text, "because ") ||
		strings.Contains(text, " is low") ||
		!strings.Contains(text, " was low on memory") {
		t.Fatalf("want the cause in past tense, got %q", text)
	}
}

func TestResolveOmitsCauseForSelfOrNone(t *testing.T) {
	p := badRollout()
	p.State, p.Resolved = incident.Resolved, at(42, 0)
	p.Opened = at(0, 0)
	for name, cause := range map[string]*rootcause.CauseRecord{
		"none": nil,
		"self": {Root: p.Root, Rule: "self"},
	} {
		t.Run(name, func(t *testing.T) {
			p.Cause = cause
			_, got := resolveNote(caseFacts{p: p, now: at(43, 0)})
			if text := got[len(got)-1].text; strings.Contains(
				text, "because") {
				t.Fatalf("no cause expected, got %q", text)
			}
		})
	}
}

func TestPastTenseMovesTheVerb(t *testing.T) {
	cases := map[string]string{
		"node n3 is low on memory":    "node n3 was low on memory",
		"pods are pending":            "pods were pending",
		"it cannot pull its image":    "it could not pull its image",
		"nothing here changes a verb": "nothing here changes a verb",
	}
	for in, want := range cases {
		if got := pastTense(in); got != want {
			t.Fatalf("pastTense(%q) = %q, want %q", in, got, want)
		}
	}
}
