package twilio

import (
	"net/http"
	"net/url"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// smsLimit is the longest SMS body Twilio accepts, in characters.
const smsLimit = 1600

// TestContract pins the exact Messages API form request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTwilio(map[string]interface{}{
		"accountSid": "AC123", "authToken": "tok-secret",
		"from": "+12025550199", "to": "+12025550100",
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
	if form.Get("From") != "+12025550199" ||
		form.Get("To") != "+12025550100" {
		t.Fatalf("From/To missing: %v", form)
	}
	body := form.Get("Body")
	if body == "" || utf8.RuneCountInString(body) > smsLimit {
		t.Fatalf("Body is %d runes", utf8.RuneCountInString(body))
	}
	providertest.AssertOneLeadingEmoji(t, body)
	hr := http.Request{Header: reqs[0].Header}
	user, pass, ok := hr.BasicAuth()
	if !ok || user != "AC123" || pass != "tok-secret" {
		t.Fatalf("basic auth = %q %q %v", user, pass, ok)
	}
}
