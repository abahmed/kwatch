package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func runTable(t *testing.T, tests []struct{ name, in, want string }) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Credentials(tc.in))
		})
	}
}

func TestRedactAuthorizationAnyScheme(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"digest", "Authorization: Digest response=abcdef0123456789",
			"Authorization: Digest [redacted]"},
		{"aws sigv4", "Authorization: AWS4-HMAC-SHA256 Credential=AK/2024",
			"Authorization: AWS4-HMAC-SHA256 [redacted]"},
		{"no scheme", "Authorization: a1b2c3d4e5f6",
			"Authorization: [redacted]"},
		{"json quoted", `{"Authorization": "Negotiate Y2hhbGxlbmdl1"}`,
			`{"Authorization": "Negotiate [redacted]"}`},
		{"already redacted", "Authorization: Bearer [redacted]",
			"Authorization: Bearer [redacted]"},
		{"prose stays", "authorization: failed for user alice",
			"authorization: failed for user alice"},
		{"no value", "authorization: ", "authorization: "},
	})
}

func TestRedactEscapedJSONKeys(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"string", `{\"password\":\"hunter2\"}`,
			`{\"password\":\"[redacted]\"}`},
		{"spaced", `{\"api_key\": \"abc def\", \"n\": 1}`,
			`{\"api_key\": \"[redacted]\", \"n\": 1}`},
		{"number", `{\"token\":12345}`, `{\"token\":\"[redacted]\"}`},
		{"descriptive key", `{\"token_ttl\":\"3600\"}`,
			`{\"token_ttl\":\"3600\"}`},
		{"other key", `{\"user\":\"bob\"}`, `{\"user\":\"bob\"}`},
	})
}

func TestRedactEnvNameValuePairs(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"yaml pair", "- name: DB_PASSWORD\n  value: s3cret",
			"- name: DB_PASSWORD\n  value: [redacted]"},
		{"quoted yaml", "name: \"API_TOKEN\"\nvalue: \"abc def\"",
			"name: \"API_TOKEN\"\nvalue: [redacted]"},
		{"json pair", `{"name":"DB_PASSWORD","value":"x1"}`,
			`{"name":"DB_PASSWORD","value":"[redacted]"}`},
		{"plain env", "name: LOG_LEVEL\nvalue: debug",
			"name: LOG_LEVEL\nvalue: debug"},
		{"file path key", "name: DB_PASSWORD_FILE\nvalue: /etc/pw",
			"name: DB_PASSWORD_FILE\nvalue: /etc/pw"},
		{"valueFrom stays", "name: DB_PASSWORD\n  valueFrom:\n    x: y",
			"name: DB_PASSWORD\n  valueFrom:\n    x: y"},
	})
}

func TestRedactChatWebhookURLs(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"slack", "POST https://hooks.slack.com/services/T01/B02/xYz123 failed",
			"POST https://hooks.slack.com/services/[redacted] failed"},
		{"discord", "https://discord.com/api/webhooks/123456/AbC-d_e.f",
			"https://discord.com/api/webhooks/[redacted]"},
		{"discord versioned", "https://discordapp.com/api/v10/webhooks/1/Tok",
			"https://discordapp.com/api/v10/webhooks/[redacted]"},
		{"slack site", "see https://slack.com/help", "see https://slack.com/help"},
	})
}

func TestRedactTelegramBotToken(t *testing.T) {
	tok := "123456789:" + strings.Repeat("Ab1_-", 7)
	runTable(t, []struct{ name, in, want string }{
		{"url", "GET https://api.telegram.org/bot" + tok + "/sendMessage",
			"GET https://api.telegram.org/bot[redacted]/sendMessage"},
		{"short", "bot12345:abc", "bot12345:abc"},
		{"robot word", "robot42:" + strings.Repeat("a", 40),
			"robot42:" + strings.Repeat("a", 40)},
	})
}

func TestRedactDSNWithoutScheme(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"mysql", "dial user:p@ss:w0rd@tcp(db.local:3306)/app failed",
			"dial user:[redacted]@tcp(db.local:3306)/app failed"},
		{"unix", "u:pw@unix(/run/my.sock)/db", "u:[redacted]@unix(/run/my.sock)/db"},
		{"no password", "host@tcp(1.2.3.4)", "host@tcp(1.2.3.4)"},
	})
}

func TestRedactKeyWordWithoutSeparator(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"apikey", "using apikey Zx81Qw9Lm2 now", "using apikey [redacted] now"},
		{"api key", "api key Zx81Qw9Lm2", "api key [redacted]"},
		{"prose", "apikey not found", "apikey not found"},
		{"prose2", "no api key provided", "no api key provided"},
	})
}

func TestRedactMoreTokenPrefixes(t *testing.T) {
	secret40 := strings.Repeat("aB3/", 10)
	runTable(t, []struct{ name, in, want string }{
		{"google oauth", "tok " + "ya" + "29." + strings.Repeat("a1_", 12),
			"tok [redacted]"},
		{"npm", "x " + "npm" + "_" + strings.Repeat("a1", 18) + " y",
			"x [redacted] y"},
		{"sendgrid", "k " + "S" + "G." + strings.Repeat("a1", 11) + "." +
			strings.Repeat("b2", 11), "k [redacted]"},
		{"aws pair", "AK" + "IA" + "ABCDEFGHIJKLMNOP " + secret40 + " ok",
			"[redacted] [redacted] ok"},
		{"npm word", "npm_config_cache is set", "npm_config_cache is set"},
		{"sg word", "SG.short", "SG.short"},
		{"aws key alone", "AK" + "IA" + "ABCDEFGHIJKLMNOP ok",
			"[redacted] ok"},
	})
}
