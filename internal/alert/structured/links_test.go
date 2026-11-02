package structured

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The structured Note is the plain paragraph and carries no dashboard
// link; the markdown rendering of the blocks does.
func TestNewIncidentKeepsLinksOutOfNote(t *testing.T) {
	p := NewIncident("dev", providertest.Linked())
	for _, line := range []string{providertest.CauseLink,
		providertest.EarlierLink, providertest.IncidentLink} {
		if strings.Contains(p.Note, line) {
			t.Fatalf("note carries %q: %q", line, p.Note)
		}
		if !strings.Contains(p.Markdown, line) {
			t.Fatalf("markdown lacks %q: %q", line, p.Markdown)
		}
	}
}
