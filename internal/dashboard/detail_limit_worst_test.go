package dashboard

import (
	"fmt"
	"strings"
	"testing"
)

// worstDetail fills every list of a detail far past its bound with
// long texts, the parts that alone used to pass the limit included.
func worstDetail() IncidentDetail {
	pad := strings.Repeat("y", 1500)
	at := testEpoch
	d := IncidentDetail{IncidentListItem: IncidentListItem{ID: "w",
		Title: pad, RootCause: pad}, Fix: pad}
	d.Why = &WhyDetail{AlertLevelReason: pad, StillOpenReason: pad}
	d.Ack = &AckDetail{Note: pad, On: "Deployment/shop/app"}
	d.BlastRadius = pad
	row := ContainerRow{Namespace: "shop", Pod: "p", Container: "c",
		Image: pad}
	for i := range 60 {
		d.Transitions = append(d.Transitions, Transition{At: at,
			Kind: "state changed", From: pad[:200], To: pad[:200]})
		d.Phases = append(d.Phases, Phase{State: "failing", From: at})
		d.FixSteps = append(d.FixSteps, FixStep{Text: pad,
			Command: "kubectl " + pad[:500]})
		d.CausedBy = append(d.CausedBy, RelatedIncident{
			ID: fmt.Sprint("c", i), Title: pad})
		d.Causing = append(d.Causing, RelatedIncident{
			ID: fmt.Sprint("e", i), Title: pad})
		d.Pager = append(d.Pager, PagerRow{Provider: "pd", State: "open",
			Reason: pad})
		d.Sent = append(d.Sent, SentMessage{Provider: "slack", At: at,
			Text: pad + pad})
		d.Timeline = append(d.Timeline, Event{At: at, Text: pad})
		d.CapturedLogLines = append(d.CapturedLogLines, pad)
		d.Changes = append(d.Changes, ChangeItem{Summary: pad,
			Fields: []FieldDiff{{Path: "data.x", Before: pad,
				After: pad}}})
		d.Affected = append(d.Affected, AffectedObject{Name: pad,
			Commands: []string{pad}})
		d.Containers = append(d.Containers, row)
		d.Deliveries = append(d.Deliveries, DeliveryAttempt{Error: pad})
		d.Warnings = append(d.Warnings, RecentWarning{Message: pad})
		d.SameTime = append(d.SameTime, SameTimeItem{Title: pad})
		d.ChangedBeforeRecovery = append(d.ChangedBeforeRecovery,
			ChangeItem{Summary: pad})
	}
	d.Moved = &MovedLinks{Into: &IncidentRef{ID: "x", Title: pad}}
	for i := range 20 {
		snap := BreakSnapshot{At: at, Reason: "got worse"}
		for range 30 {
			snap.Containers = append(snap.Containers, row)
		}
		d.Snapshots = append(d.Snapshots, snap)
		d.Moved.From = append(d.Moved.From, IncidentRef{
			ID: fmt.Sprint("m", i), Title: pad})
	}
	d.Snapshot = &d.Snapshots[len(d.Snapshots)-1]
	return d
}

func TestFitDetailFitsTheWorstCase(t *testing.T) {
	d := worstDetail()
	if detailSize(&d) <= 20*detailBudget {
		t.Fatalf("fixture is only %d bytes", detailSize(&d))
	}
	fitDetail(&d, detailBudget)
	if n := detailSize(&d); n > detailBudget {
		t.Fatalf("detail is %d bytes, over %d", n, detailBudget)
	}
	tr := d.Truncated
	if tr == nil {
		t.Fatal("nothing counted as left out")
	}
	cuts := map[string]int{"transitions": tr.Transitions,
		"snapshots": tr.Snapshots, "snapshotRows": tr.SnapshotRows,
		"phases": tr.Phases, "sent": tr.Sent, "movedFrom": tr.MovedFrom,
		"texts": tr.Texts}
	for name, n := range cuts {
		if n == 0 {
			t.Errorf("%s: nothing cut: %+v", name, *tr)
		}
	}
	if d.Snapshot != nil && len(d.Snapshots) > 0 &&
		d.Snapshot.At != d.Snapshots[len(d.Snapshots)-1].At {
		t.Error("snapshot is not the latest kept")
	}
}

// Even the record alone, with nothing else, fits: it used to be left
// for writeJSON to refuse.
func TestFitDetailCutsTheRecordAlone(t *testing.T) {
	full := worstDetail()
	d := IncidentDetail{IncidentListItem: full.IncidentListItem,
		Transitions: full.Transitions, Snapshots: full.Snapshots,
		Snapshot: full.Snapshot, Phases: full.Phases,
		FixSteps: full.FixSteps, Why: full.Why, Ack: full.Ack}
	d.CausedBy, d.Causing = full.CausedBy, full.Causing
	d.Sent, d.Pager = full.Sent, full.Pager
	fitDetail(&d, detailBudget)
	if n := detailSize(&d); n > detailBudget {
		t.Fatalf("detail is %d bytes, over %d", n, detailBudget)
	}
	if d.Truncated == nil || d.Truncated.SnapshotRows == 0 {
		t.Errorf("truncated = %+v", d.Truncated)
	}
}

func TestShortenTextsMarksASentTextCut(t *testing.T) {
	d := IncidentDetail{Sent: []SentMessage{{Text: strings.Repeat(
		"z", 900)}}}
	shortenTexts(&d, &DetailTruncation{}, 100)
	if !d.Sent[0].Cut || len([]rune(d.Sent[0].Text)) != 100 {
		t.Errorf("sent = %+v", d.Sent[0])
	}
}
