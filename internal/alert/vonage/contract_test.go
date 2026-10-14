package vonage

import (
	"net/url"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// smsLimit is the longest SMS body Vonage accepts, in characters.
const smsLimit = 1600

// TestContract pins the exact SMS API form request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewVonage(map[string]interface{}{
		"apiKey": "key-secret", "apiSecret": "tok-secret",
		"from": "kwatch", "to": "+12025550100",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets:    []string{"key-secret", "tok-secret"},
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
	if form.Get("from") != "kwatch" ||
		form.Get("to") != "+12025550100" {
		t.Fatalf("from/to missing: %v", form)
	}
	body := form.Get("text")
	if body == "" || utf8.RuneCountInString(body) > smsLimit {
		t.Fatalf("text is %d runes", utf8.RuneCountInString(body))
	}
	providertest.AssertOneLeadingEmoji(t, body)
	if form.Get("api_key") != "key-secret" ||
		form.Get("api_secret") != "tok-secret" {
		t.Fatalf("credentials missing from the form: %v", form)
	}
}
