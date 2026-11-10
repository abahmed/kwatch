package redact

import "testing"

func TestRedactServiceCredentialShapes(t *testing.T) {
	const hex32 = "0123456789abcdef0123456789abcdef"
	const teamsPath = "a1b2-c3d4@e5f6/IncomingWebhook/abc123/def456"
	const tg = "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw0"
	runTable(t, []struct{ name, in, want string }{
		{"slack app token", "token xapp-1-" + "A0123456789-1234567890123-abcdef01234",
			"token [redacted]"},
		{"slack legacy token", "got xoxs-" + "1234567890-abcdefghij1234 ok",
			"got [redacted] ok"},
		{"slack rotating token", "got xoxe.xoxp-" + "1-Mi0yLTEyMzQ1Njc4OTA ok",
			"got [redacted] ok"},
		{"slack refresh token", "got xoxe-" + "1-My0xLTEyMzQ1Njc4OTA ok",
			"got [redacted] ok"},
		{"telegram bare", "bot 123456789:" + tg + " ready",
			"bot [redacted] ready"},
		{"teams old webhook",
			"POST https://outlook.office.com/webhook/" + teamsPath + " failed",
			"POST https://outlook.office.com/webhook/[redacted] failed"},
		{"teams new webhook",
			"POST https://contoso.webhook.office.com/webhookb2/" +
				teamsPath + " failed",
			"POST https://contoso.webhook.office.com/webhookb2/[redacted] failed"},
		{"zapier", "POST https://hooks.zapier.com/hooks/catch/123456/abcdef/ x",
			"POST https://hooks.zapier.com/hooks/catch/[redacted] x"},
		{"stripe webhook secret",
			"secret whsec_" + "MbCdEfGh1234567890AbCdEfGh12 set",
			"secret [redacted] set"},
		{"stripe live", "key sk_live_" + "4eC39HqLyjWDarjtT1zdp7dc",
			"key [redacted]"},
		{"twilio key", "sid SK" + hex32 + " used", "sid [redacted] used"},
		{"pypi", "pypi-AgEIcHlwaS5vcmcCJDEyMzQ1Njc4OTBhYmNkZWYxMjM0NTY3 up",
			"[redacted] up"},
		{"pagerduty key value", "routing_key=R0UTINGKEY12345 sent",
			"routing_key=[redacted] sent"},
		{"pagerduty json", `{"routing_key":"R0UTINGKEY12345","a":1}`,
			`{"routing_key":"[redacted]","a":1}`},
	})
}

func TestRedactServiceShapesKeepOrdinaryLines(t *testing.T) {
	for _, line := range []string{
		"started at 12:34:56 in 1234567890:ok",
		"ratio 20240101:abc took 5s",
		"pod web-1 SK123 restarted",
		"pip install pypi-mirror",
		"opening https://outlook.office.com/mail/inbox",
		"zapier hooks are disabled",
		"xapp-config loaded, xoxo",
		"whsec_ is the prefix",
	} {
		if got := Credentials(line); got != line {
			t.Errorf("changed %q to %q", line, got)
		}
	}
}
