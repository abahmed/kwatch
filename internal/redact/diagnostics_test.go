package redact

import (
	"strings"
	"testing"
)

func TestDiagnosticsRemovesAddressesAndCredentials(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"url", "posting to https://hooks.slack.com/services/T0/B1/x ok",
			"posting to <url> ok"},
		{"email", "smtp to ops@example.com failed",
			"smtp to <email> failed"},
		{"ipv4", "dial tcp 10.0.0.12:443: refused",
			"dial tcp <ip>:443: refused"},
		{"public ipv4", "from 8.8.8.8", "from <ip>"},
		{"ipv6", "addr 2001:db8:0:0:0:0:2:1 up", "addr <ip> up"},
		{"time kept", "at 12:30:45 UTC", "at 12:30:45 UTC"},
		{"pod name kept", "pod kwatch-7d9f-abcde ready",
			"pod kwatch-7d9f-abcde ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Diagnostics(tc.in); got != tc.want {
				t.Fatalf("Diagnostics(%q) = %q, want %q",
					tc.in, got, tc.want)
			}
		})
	}
}

func TestDiagnosticsKeepsCredentialRedaction(t *testing.T) {
	got := Diagnostics("password: hunter2 token=abcdef123456")
	for _, secret := range []string{"hunter2", "abcdef123456"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q left in %q", secret, got)
		}
	}
}
