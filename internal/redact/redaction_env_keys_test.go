package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactEnvironmentStyleKeys(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"env password", "DB_PASSWORD=hunter2", "DB_PASSWORD=[redacted]"},
		{"env token", "export GITHUB_TOKEN=ghp_abc123",
			"export GITHUB_TOKEN=[redacted]"},
		{"env api key", "MY_API_KEY: k-123", "MY_API_KEY: [redacted]"},
		{"env plural", "GITHUB_TOKENS=a,b",
			"GITHUB_TOKENS=[redacted],b"},
		{"cli flag", "--db-password=s3cr3t", "--db-password=[redacted]"},
		{"json prefixed key", `{"db_password":"hunter2","user":"a"}`,
			`{"db_password":"[redacted]","user":"a"}`},
		{"json escaped quote", `{"password":"a\"b c"}`,
			`{"password":"[redacted]"}`},
		{"quoted value with spaces", `password="two words" next`,
			`password=[redacted] next`},
		{"single quoted value", `secret: 'two words' next`,
			`secret: [redacted] next`},
		{"query prefixed key", "GET /x?x_api_key=abc",
			"GET /x?x_api_key=[redacted]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Credentials(tc.in))
		})
	}
}

func TestRedactLeavesProseAndLookalikeWords(t *testing.T) {
	for _, in := range []string{
		"password policy requires 12 characters",
		"the token was rotated yesterday",
		"tokenizer=fast",
		"mysecretary: alice",
		`{"tokens_total":5}`,
	} {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, in, Credentials(in))
		})
	}
}
