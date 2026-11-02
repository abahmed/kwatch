package sendgrid

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A Pro message is mailed with the same HTML layout SMTP sends, beside
// the plain text; any other message stays plain text only.
func TestSendgridMailsProHTML(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSendgrid(map[string]interface{}{
		"apiKey": "key-secret", "from": "kwatch@example.com",
		"to": "ops@example.com",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	ctx := context.Background()
	if err := c.SendIncident(ctx, providertest.ProEvidence()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text, page := contentBodies(t, rec)
	if !strings.Contains(page, "<h3>What kwatch found</h3>") ||
		!strings.Contains(text, "What kwatch found") {
		t.Fatalf("Pro bodies missing: text %.80q html %.80q", text, page)
	}
	if err := c.SendIncident(ctx, providertest.Announce()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if _, page := contentBodies(t, rec); page != "" {
		t.Fatalf("plain message got HTML: %.80q", page)
	}
}

// contentBodies reads the text/plain and text/html content, and checks
// that text/plain comes first, as SendGrid requires.
func contentBodies(
	t *testing.T, rec *providertest.Recorder,
) (string, string) {
	t.Helper()
	content, _ := rec.Last(t).JSON(t)["content"].([]any)
	var text, page string
	for i, c := range content {
		part := c.(map[string]any)
		switch part["type"] {
		case "text/plain":
			if i != 0 {
				t.Fatal("text/plain must come first")
			}
			text, _ = part["value"].(string)
		case "text/html":
			page, _ = part["value"].(string)
		}
	}
	return text, page
}
