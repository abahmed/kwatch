package teamsworkflow

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TeamsWorkflow renders Markdown, so pod and log text in the narrative
// is escaped: it never turns into formatting or a live link.
func TestTeamsWorkflowEscapesMarkdownInNarrative(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeamsWorkflow(
		map[string]interface{}{"webhook": rec.URL()}, "",
		rec.Dependencies())
	msg := providertest.Announce()
	msg.Note = "🔴 pod *api* said _boom_ see [docs](http://evil.example)"
	msg.Doc = nil
	if err := c.SendIncident(context.Background(), msg); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text := cardText(t, rec.Last(t).Body)
	for _, want := range []string{
		"\\*api\\*", "\\_boom\\_", "\\[docs\\]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("%q not escaped in %q", want, text)
		}
	}
}
