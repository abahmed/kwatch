package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

func TestRecoveredNamesTheOtherRecoveries(t *testing.T) {
	lead := notification.Message{Note: "✅ A is healthy again.",
		Title: "A is healthy again."}
	b := notification.Message{Title: "B is healthy again."}
	c := notification.Message{Title: "C is healthy again."}

	got := Recovered(lead, []notification.Message{b, c})

	if !strings.Contains(got.Note, "same moment: B is healthy again. C") {
		t.Fatalf("note = %q", got.Note)
	}
	last := got.Doc[len(got.Doc)-1]
	if last.Kind != notification.Bullet || last.Text() != c.Title {
		t.Fatalf("doc = %+v", got.Doc)
	}
	if len(got.Doc) != 3 || got.Title != lead.Title {
		t.Fatalf("lead must stay the first line: %+v", got)
	}
}

func TestRecoveredAloneIsUnchanged(t *testing.T) {
	lead := notification.Message{Note: "x", Title: "A is healthy again."}
	if got := Recovered(lead, nil); got.Note != "x" || got.Doc != nil {
		t.Fatalf("got %+v", got)
	}
}

// A carried recovery that speaks of its listing's subject keeps the
// line naming the root it ended on.
func TestRecoveredKeepsTheLastAffectedLine(t *testing.T) {
	lead := notification.Message{Title: "A is healthy again."}
	b := notification.Message{Title: "Node n1 is ready again.",
		Lines: []string{"It was down for 5 minutes.",
			"Last affected: deployment shop/cart."}}

	got := Recovered(lead, []notification.Message{b})

	want := "Node n1 is ready again. Last affected: deployment shop/cart."
	if last := got.Doc[len(got.Doc)-1]; last.Text() != want {
		t.Fatalf("bullet = %q", last.Text())
	}
}

// Deletions of one tick are listed under their own heading, never as
// recoveries.
func TestDeletedNamesTheOtherDeletions(t *testing.T) {
	lead := notification.Message{Note: "A was deleted.",
		Title: "A was deleted."}
	b := notification.Message{Title: "B was deleted."}

	got := Deleted(lead, []notification.Message{b})

	if !strings.Contains(got.Note, "Deleted at the same moment: B was") ||
		strings.Contains(got.Note, "Recovered") {
		t.Fatalf("note = %q", got.Note)
	}
	if last := got.Doc[len(got.Doc)-1]; last.Text() != b.Title {
		t.Fatalf("doc = %+v", got.Doc)
	}
}

func TestDeletedAloneIsUnchanged(t *testing.T) {
	lead := notification.Message{Note: "x", Title: "A was deleted."}
	if got := Deleted(lead, nil); got.Note != "x" || got.Doc != nil {
		t.Fatalf("got %+v", got)
	}
}
