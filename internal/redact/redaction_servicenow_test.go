package redact

import (
	"strings"
	"testing"
)

// Fake ServiceNow credentials are joined from pieces so secret scanners do
// not take the fixtures for real ones.
func TestRedactServiceNowCredentials(t *testing.T) {
	secret := strings.Join([]string{"Zx9", "kQ2", "mPa", "7Lw4"}, "")
	token := strings.Join([]string{"aBcD", "1234", "eFgH", "5678", "iJkL"},
		"")
	basic := strings.Join([]string{"a3dhdGNo", "OnMzY3JldA=="}, "")
	runTable(t, []struct{ name, in, want string }{
		{"token form", "grant_type=client_credentials&client_id=kw&" +
			"client_secret=" + secret,
			"grant_type=client_credentials&client_id=kw&" +
				"client_secret=[redacted]"},
		{"token answer", `{"access_token":"` + token +
			`","refresh_token":"` + token + `","expires_in":1799}`,
			`{"access_token":"[redacted]","refresh_token":"[redacted]",` +
				`"expires_in":1799}`},
		{"bearer header", "Authorization: Bearer " + token,
			"Authorization: Bearer [redacted]"},
		{"basic header", "Authorization: Basic " + basic,
			"Authorization: Basic [redacted]"},
		{"settings", "clientSecret: " + secret,
			"clientSecret: [redacted]"},
		{"password setting", "password: " + secret,
			"password: [redacted]"},
	})
}

func TestRedactServiceNowKeepsRecordIdentifiers(t *testing.T) {
	for _, line := range []string{
		"PATCH /api/now/table/incident/9d385017c611228701d22104cc95c371",
		"correlation_id=kwatchkeyd0603fe338e68c0e^active=true",
		"incident INC0010023 resolved with close_code Solution provided",
	} {
		if got := Credentials(line); got != line {
			t.Errorf("changed %q to %q", line, got)
		}
	}
}
