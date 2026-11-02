package ses

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A Pro message is mailed with the same HTML layout SMTP sends, beside
// the plain text; any other message stays plain text only.
func TestSesMailsProHTML(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSes(map[string]interface{}{
		"accessKeyId": testKeyID, "secretAccessKey": testSecret,
		"from": "kwatch@example.com", "to": "ops@example.com",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	ctx := context.Background()
	if err := c.SendIncident(ctx, providertest.ProEvidence()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text, page := formBodies(t, rec, "Message.Body.Text.Data",
		"Message.Body.Html.Data")
	if !strings.Contains(page, "<h3>What kwatch found</h3>") ||
		!strings.Contains(text, "What kwatch found") {
		t.Fatalf("Pro bodies missing: text %.80q html %.80q", text, page)
	}
	if err := c.SendIncident(ctx, providertest.Announce()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if _, page := formBodies(t, rec, "Message.Body.Text.Data",
		"Message.Body.Html.Data"); page != "" {
		t.Fatalf("plain message got HTML: %.80q", page)
	}
}

func formBodies(
	t *testing.T, rec *providertest.Recorder, text, html string,
) (string, string) {
	t.Helper()
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	return form.Get(text), form.Get(html)
}
