package messagebird

import (
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// smsLimit is the longest SMS body MessageBird accepts, in characters.
const smsLimit = 1600

// TestContract pins the exact Messages API request.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewMessagebird(map[string]interface{}{
		"accessKey": "tok-secret", "from": "kwatch",
		"to": "+12025550100",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers:    []string{"Authorization"},
		Secrets:    []string{"tok-secret"},
		LimitBytes: smsLimit,
		Check:      checkSMSShape,
	})
}

func checkSMSShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	if got := reqs[0].Header.Get("Authorization"); got !=
		"AccessKey tok-secret" {
		t.Fatalf("Authorization = %q", got)
	}
	p := reqs[0].JSON(t)
	rcpt, _ := p["recipients"].([]any)
	if p["originator"] != "kwatch" || len(rcpt) != 1 ||
		rcpt[0] != "+12025550100" {
		t.Fatalf("originator or recipients missing: %v", p)
	}
	body, _ := p["body"].(string)
	if body == "" || utf8.RuneCountInString(body) > smsLimit {
		t.Fatalf("body is %d runes", utf8.RuneCountInString(body))
	}
	providertest.AssertOneLeadingEmoji(t, body)
}
