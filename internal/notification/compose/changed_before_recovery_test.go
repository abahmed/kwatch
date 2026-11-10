package compose

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

const beforeLine = "Changed just before it recovered: "

// recoveredWith is a resolve of an incident that failed from 9:30, began
// to recover at 9:57 and resolved at 10:00, with changes linked at the
// given minutes before 10:00.
func recoveredWith(minutesBefore ...int) incident.Decision {
	d := runbookDecision(incident.Resolve, "OOMKilled")
	end := d.Incident.Resolved
	d.Incident.Opened = end.Add(-30 * time.Minute)
	d.Incident.RecoveringSince = end.Add(-3 * time.Minute)
	d.Incident.Members = nil
	for _, m := range minutesBefore {
		d.Incident.Journal.Changes = append(d.Incident.Journal.Changes,
			inventory.Change{Entity: workload("payments"),
				At: end.Add(-time.Duration(m) * time.Minute),
				Fields: []inventory.FieldChange{{Path: "image",
					Before: "v" + strconv.Itoa(m),
					After:  "v" + strconv.Itoa(m+1)}}})
	}
	return d
}

// A resolve names, newest first and at most two, the changes made
// shortly before the incident began to recover.
func TestResolveNamesChangesJustBeforeRecovery(t *testing.T) {
	d := recoveredWith(5, 8, 10, 25)

	note := Writer{}.Write(d, d.Incident.Resolved).Note

	at := strings.Index(note, beforeLine)
	if at < 0 {
		t.Fatalf("no line in %q", note)
	}
	line := note[at:]
	first, second := strings.Index(line, "09:55"), strings.Index(line,
		"09:52")
	if first < 0 || second < first || strings.Contains(line, "09:50") {
		t.Fatalf("want the two newest, newest first: %q", line)
	}
	for _, word := range []string{"fixed", "because"} {
		if strings.Contains(line, word) {
			t.Fatalf("the line must not claim a fix: %q", line)
		}
	}
}

// Changes after the recovery began, or older than the window, or made
// before the failures began, are not named; neither is the fix.
func TestResolveSkipsChangesOutsideTheWindow(t *testing.T) {
	d := recoveredWith(1, 25)
	d.Incident.Opened = d.Incident.Resolved.Add(-10 * time.Minute)
	d.Incident.Journal.Changes = append(d.Incident.Journal.Changes,
		inventory.Change{Entity: workload("payments"),
			At: d.Incident.Resolved.Add(-12 * time.Minute)})

	note := Writer{}.Write(d, d.Incident.Resolved).Note

	if strings.Contains(note, beforeLine) {
		t.Fatalf("nothing is in the window: %q", note)
	}

	fixed := recoveredWith(5)
	fix := fixed.Incident.Journal.Changes[0]
	f := caseFacts{p: fixed.Incident, fix: &fix}
	if got := changedBeforeRecoverySentences(f); got != nil {
		t.Fatalf("the fix is said already: %+v", got)
	}
}

// The grouped recovery keeps the line beside the carried lead.
func TestRecoveredKeepsTheChangedBeforeLine(t *testing.T) {
	d := recoveredWith(5)
	follower := Writer{}.Write(d, d.Incident.Resolved)
	lead := notification.Message{Title: "A is healthy again."}

	got := Recovered(lead, []notification.Message{follower})

	if !strings.Contains(got.Note, beforeLine) {
		t.Fatalf("note = %q", got.Note)
	}
}

// A recovery with changes listed just before it is not said to be on
// its own; without such a change it still is.
func TestResolveWithChangesBeforeIsNotOnItsOwn(t *testing.T) {
	d := recoveredWith(5)
	d.Incident.Fix = incident.FixNone
	note := Writer{}.Write(d, d.Incident.Resolved).Note
	if !strings.Contains(note, beforeLine) ||
		strings.Contains(note, "on its own") {
		t.Fatalf("note = %q", note)
	}

	quiet := recoveredWith(25)
	quiet.Incident.Fix = incident.FixNone
	note = Writer{}.Write(quiet, quiet.Incident.Resolved).Note
	if !strings.Contains(note, "recovered on its own") {
		t.Fatalf("note = %q", note)
	}
}
