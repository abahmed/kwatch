package mailgun

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Mailgun messages form request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewMailgun(map[string]interface{}{
		"apiKey": "key-secret", "domain": "mg.example.com",
		"from": "kwatch@example.com",
		"to":   "ops@example.com, oncall@example.com",
		"url":  rec.URL(),
	}, "dev", rec.Dependencies())
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
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/mg.example.com/messages" {
		t.Fatalf("requests = %+v", reqs)
	}
	form, err := url.ParseQuery(string(reqs[0].Body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("from") != "kwatch@example.com" ||
		strings.Join(form["to"], ",") !=
			"ops@example.com,oncall@example.com" {
		t.Fatalf("from or to wrong: %v", form)
	}
	subject := form.Get("subject")
	if subject == "" || strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("subject must be one line: %q", subject)
	}
	providertest.AssertOneLeadingEmoji(t, subject)
	if strings.TrimSpace(form.Get("text")) == "" {
		t.Fatal("text body is empty")
	}
	hr := http.Request{Header: reqs[0].Header}
	user, pass, ok := hr.BasicAuth()
	if !ok || user != "api" || pass != "key-secret" {
		t.Fatalf("basic auth = %q %q %v", user, pass, ok)
	}
}
