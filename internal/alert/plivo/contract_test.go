package plivo

import (
	"net/http"
	"net/url"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// smsLimit is the longest SMS body Plivo accepts, in characters.
const smsLimit = 1600

// TestContract pins the exact Message API form request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewPlivo(map[string]interface{}{
		"authId": "MA123", "authToken": "tok-secret",
		"from": "kwatch", "to": "+12025550100",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers:    []string{"Authorization"},
		Secrets:    []string{"tok-secret"},
		LimitBytes: smsLimit,
		Check:      checkFormShape,
	})
}

func checkFormShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	form, err := url.ParseQuery(string(reqs[0].Body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("src") != "kwatch" ||
		form.Get("dst") != "+12025550100" {
		t.Fatalf("src/dst missing: %v", form)
	}
	body := form.Get("text")
	if body == "" || utf8.RuneCountInString(body) > smsLimit {
		t.Fatalf("text is %d runes", utf8.RuneCountInString(body))
	}
	providertest.AssertOneLeadingEmoji(t, body)
	hr := http.Request{Header: reqs[0].Header}
	user, pass, ok := hr.BasicAuth()
	if !ok || user != "MA123" || pass != "tok-secret" {
		t.Fatalf("basic auth = %q %q %v", user, pass, ok)
	}
}
