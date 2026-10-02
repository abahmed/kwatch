package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fake builds a token fixture from parts so secret scanners do not flag
// the test file: prefix followed by n copies of "a1".
func fake(prefix string, n int) string {
	return prefix + strings.Repeat("a1", n)
}

func TestRedactBareTokenPrefixes(t *testing.T) {
	tests := []struct{ name, token string }{
		{"github personal", fake("gh"+"p_", 18)},
		{"github oauth", fake("gh"+"o_", 18)},
		{"github user", fake("gh"+"u_", 18)},
		{"github server", fake("gh"+"s_", 18)},
		{"github refresh", fake("gh"+"r_", 18)},
		{"github fine grained", fake("github"+"_pat_", 20)},
		{"slack bot", fake("xo"+"xb-", 10)},
		{"slack app", fake("xo"+"xa-", 10)},
		{"slack user", fake("xo"+"xp-", 10)},
		{"slack refresh", fake("xo"+"xr-", 10)},
		{"gitlab", fake("gl"+"pat-", 10)},
		{"google", "AI" + "za" + strings.Repeat("b", 35)},
		{"stripe live", fake("sk"+"_live_", 12)},
		{"stripe test", fake("sk"+"_test_", 12)},
		{"openai", fake("s"+"k-", 12)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Credentials("clone failed with " + tc.token + " today")
			assert.Equal(t, "clone failed with [redacted] today", got)
		})
	}
}

func TestRedactAuthorizationHeaderSchemes(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"token", "Authorization: token " + fake("v", 4),
			"Authorization: token [redacted]"},
		{"bearer", "authorization: Bearer " + fake("v", 4),
			"authorization: Bearer [redacted]"},
		{"basic", "Authorization: Basic dXNlcjpwYXNz",
			"Authorization: Basic [redacted]"},
		{"basic non base64", "Authorization=basic opaque",
			"Authorization=basic [redacted]"},
		{"bare basic credential", "sent Basic dXNlcjpwYXNz",
			"sent Basic [redacted]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Credentials(tc.in))
		})
	}
}

func TestRedactKeyValueVariants(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plural env key", "GITHUB_TOKENS=abc123",
			"GITHUB_TOKENS=[redacted]"},
		{"json number", `{"token": 12345}`, `{"token": "[redacted]"}`},
		{"json bool", `{"secret":true}`, `{"secret":"[redacted]"}`},
		{"query plural", "/x?tokens=abc", "/x?tokens=[redacted]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Credentials(tc.in))
		})
	}
}

// These describe a secret without holding one, so they stay visible.
func TestRedactKeepsDescriptiveKeysAndProse(t *testing.T) {
	for _, in := range []string{
		"Running basic health checks",
		"basic auth is disabled",
		"token_ttl=300",
		"secret_name: app-db",
		"token-expiry-seconds: 30",
		"secret-manager: vault",
		"DB_PASSWORD_FILE=/run/pw",
		"api_key_id=42",
		"/x?token_ref=main",
		`{"tokens_total":5}`,
		`{"secret_name":"app-db"}`,
		"pip install sk-learn",
		"the ghp_ prefix marks GitHub tokens",
	} {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, in, Credentials(in))
		})
	}
}
