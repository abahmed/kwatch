package structured

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The structured Note is the plain paragraph and carries no dashboard
// link; the markdown rendering of the blocks links each label.
func TestNewIncidentKeepsLinksOutOfNote(t *testing.T) {
	p := NewIncident("dev", providertest.Linked())
	for _, link := range providertest.LinkSpans() {
		if strings.Contains(p.Note, link.URL) ||
			strings.Contains(p.Note, link.Text) {
			t.Fatalf("note carries %q: %q", link.Text, p.Note)
		}
		want := "[" + link.Text + "](" + link.URL + ")"
		if !strings.Contains(p.Markdown, want) {
			t.Fatalf("markdown lacks %q: %q", want, p.Markdown)
		}
	}
}
