package sendgrid

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact v3 mail/send request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSendgrid(map[string]interface{}{
		"apiKey": "key-secret", "from": "kwatch@example.com",
		"to": []interface{}{"ops@example.com", "oncall@example.com"},
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
	p := sentPayloadOf(t, reqs[0])
	if len(p.Personalizations) != 1 ||
		len(p.Personalizations[0].To) != 2 || p.From.Email == "" {
		t.Fatalf("personalizations or from missing: %+v", p)
	}
	if strings.ContainsAny(p.Subject, "\r\n") {
		t.Fatalf("subject must be one line: %q", p.Subject)
	}
	providertest.AssertOneLeadingEmoji(t, p.Subject)
	if len(p.Content) != 1 || p.Content[0].Type != "text/plain" ||
		strings.TrimSpace(p.Content[0].Value) == "" {
		t.Fatalf("content = %+v", p.Content)
	}
}

func sentPayloadOf(
	t *testing.T, r providertest.Request,
) sendgridPayload {
	t.Helper()
	var p sendgridPayload
	if err := json.Unmarshal(r.Body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}
