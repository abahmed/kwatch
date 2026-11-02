package wecom

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// Wecom renders Markdown, so pod and log text in the narrative is escaped:
// it never turns into formatting or a live link.
func TestWecomEscapesMarkdownInNarrative(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewWecom(map[string]interface{}{
		"webhook": rec.URL() + "/cgi-bin/webhook/send?key=hk",
	}, "", rec.Dependencies())
	msg := providertest.Announce()
	msg.Note = "🔴 pod *api* said _boom_ see [docs](http://evil.example)"
	msg.Doc = nil
	if err := c.SendIncident(context.Background(), msg); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	content := rec.Last(t).JSON(t)["markdown"].(map[string]any)["content"]
	text := content.(string)
	for _, want := range []string{
		"\\*api\\*", "\\_boom\\_", "\\[docs\\]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("%q not escaped in %q", want, text)
		}
	}
}
