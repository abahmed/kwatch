package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// recoveredAtTen failed from 9:30, began to recover at 9:57 and
// resolved at 10:00, with image changes at the minutes before 10:00.
func recoveredAtTen(minutesBefore ...int) Incident {
	end := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	p := Incident{Opened: end.Add(-30 * time.Minute),
		RecoveringSince: end.Add(-3 * time.Minute), Resolved: end}
	for _, m := range minutesBefore {
		p.Journal.Changes = append(p.Journal.Changes, inventory.Change{
			Entity: inventory.EntityID{Kind: kube.KindDeployment,
				Namespace: "shop", Name: "payments"},
			At: end.Add(-time.Duration(m) * time.Minute),
			Fields: []inventory.FieldChange{{Path: "spec.image",
				Before: "v1", After: "v2"}}})
	}
	return p
}

func TestChangesBeforeRecoveryKeepsTheTwoNewestInTheWindow(t *testing.T) {
	p := recoveredAtTen(1, 5, 8, 10, 25)
	got := ChangesBeforeRecovery(p, nil)
	if len(got) != MaxBeforeRecovery {
		t.Fatalf("got %d changes, want %d", len(got), MaxBeforeRecovery)
	}
	end := p.Resolved
	if !got[0].At.Equal(end.Add(-5*time.Minute)) ||
		!got[1].At.Equal(end.Add(-8*time.Minute)) {
		t.Errorf("want 9:55 then 9:52, got %v and %v",
			got[0].At, got[1].At)
	}
}

func TestChangesBeforeRecoveryLeavesOutTheFixAndTaints(t *testing.T) {
	p := recoveredAtTen(5, 8)
	fix := p.Journal.Changes[0]
	taint := inventory.Change{
		Entity: inventory.EntityID{Kind: kube.KindNode, Name: "n1"},
		At:     p.Resolved.Add(-4 * time.Minute),
		Fields: []inventory.FieldChange{{Path: "spec.taints"}}}
	p.Journal.Changes = append(p.Journal.Changes, taint)
	got := ChangesBeforeRecovery(p, &fix)
	if len(got) != 1 || !got[0].At.Equal(p.Journal.Changes[1].At) {
		t.Errorf("got %+v, want only the 9:52 change", got)
	}
}

func TestChangesBeforeRecoveryStartsAtTheOnset(t *testing.T) {
	p := recoveredAtTen(5, 12)
	p.Opened = p.Resolved.Add(-10 * time.Minute)
	got := ChangesBeforeRecovery(p, nil)
	if len(got) != 1 {
		t.Errorf("got %d changes, want the one after the onset",
			len(got))
	}
}

func TestSystemChangeIsANodeTaintOnly(t *testing.T) {
	node := inventory.EntityID{Kind: kube.KindNode, Name: "n1"}
	cases := []struct {
		name   string
		change inventory.Change
		want   bool
	}{
		{"taints", inventory.Change{Entity: node, Fields: []inventory.
			FieldChange{{Path: "spec.taints"}}}, true},
		{"labels too", inventory.Change{Entity: node, Fields: []inventory.
			FieldChange{{Path: "spec.taints"}, {Path: "labels"}}}, false},
		{"deleted", inventory.Change{Entity: node, Deleted: true,
			Fields: []inventory.FieldChange{{Path: "taints"}}}, false},
		{"no fields", inventory.Change{Entity: node}, false},
	}
	for _, c := range cases {
		if got := SystemChange(c.change); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
