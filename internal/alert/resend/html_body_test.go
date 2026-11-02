package resend

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A Pro message is mailed with the same HTML layout SMTP sends, beside
// the plain text; any other message stays plain text only.
func TestResendMailsProHTML(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewResend(map[string]interface{}{
		"apiKey": "key-secret", "from": "kwatch@example.com",
		"to": "ops@example.com",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	ctx := context.Background()
	if err := c.SendIncident(ctx, providertest.ProEvidence()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text, page := jsonBodies(t, rec)
	if !strings.Contains(page, "<h3>What kwatch found</h3>") ||
		!strings.Contains(text, "What kwatch found") {
		t.Fatalf("Pro bodies missing: text %.80q html %.80q", text, page)
	}
	if err := c.SendIncident(ctx, providertest.Announce()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if _, page := jsonBodies(t, rec); page != "" {
		t.Fatalf("plain message got HTML: %.80q", page)
	}
}

func jsonBodies(
	t *testing.T, rec *providertest.Recorder,
) (string, string) {
	t.Helper()
	body := rec.Last(t).JSON(t)
	text, _ := body["text"].(string)
	page, _ := body["html"].(string)
	return text, page
}
