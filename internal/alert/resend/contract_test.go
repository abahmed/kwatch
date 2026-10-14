package resend

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Resend emails request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewResend(map[string]interface{}{
		"apiKey": "key-secret", "from": "kwatch@example.com",
		"to": "ops@example.com, oncall@example.com",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers:    []string{"Authorization"},
		Secrets:    []string{"key-secret"},
		LimitBytes: 10000,
		Check:      checkMailShape,
	})
}

func checkMailShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	if reqs[0].Header.Get("Authorization") != "Bearer key-secret" {
		t.Fatal("Authorization must be a Bearer API key")
	}
	p := reqs[0].JSON(t)
	to, _ := p["to"].([]any)
	if p["from"] != "kwatch@example.com" || len(to) != 2 {
		t.Fatalf("from or to wrong: %v", p)
	}
	subject, _ := p["subject"].(string)
	if subject == "" || strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("subject must be one line: %q", subject)
	}
	providertest.AssertOneLeadingEmoji(t, subject)
	if text, _ := p["text"].(string); strings.TrimSpace(text) == "" {
		t.Fatal("text body is empty")
	}
}
