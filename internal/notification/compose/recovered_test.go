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
