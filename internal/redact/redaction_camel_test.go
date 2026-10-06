package redact

import "testing"

// camelCase key names, npm's _authToken and URL userinfo shapes that the
// snake_case rules missed.
func TestRedactCamelCaseKeys(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"authToken", "authToken=abc123", "authToken=[redacted]"},
		{"refreshToken", "refreshToken: x1y2", "refreshToken: [redacted]"},
		{"sessionToken", "sessionToken=s3ss", "sessionToken=[redacted]"},
		{"apiToken", "apiToken=tok99", "apiToken=[redacted]"},
		{"apiSecret", "apiSecret=shh1", "apiSecret=[redacted]"},
		{"bearerToken", "bearerToken=b3arer", "bearerToken=[redacted]"},
		{"signingKey", "signingKey=k3y", "signingKey=[redacted]"},
		{"clientSecret", "clientSecret=cs1", "clientSecret=[redacted]"},
		{"accessKey", "accessKey=ak1", "accessKey=[redacted]"},
		{"secretToken", "secretToken=st1", "secretToken=[redacted]"},
		{"privateKey", "privateKey=pk1", "privateKey=[redacted]"},
		{"dbPassword", "dbPassword=pw1", "dbPassword=[redacted]"},
		{"json", `{"authToken":"x","name":"a"}`,
			`{"authToken":"[redacted]","name":"a"}`},
		{"query", "GET /x?authToken=abc", "GET /x?authToken=[redacted]"},
		{"npm", "//registry.npmjs.org/:_authToken=npmsecret1",
			"//registry.npmjs.org/:_authToken=[redacted]"},
		{"snake access key", "access_key=ak2", "access_key=[redacted]"},
		{"signing key snake", "signing-key: k4", "signing-key: [redacted]"},
	})
}

func TestRedactCamelCaseKeysKeepDescriptions(t *testing.T) {
	same := []string{
		"tokenizer=nltk",
		"authTokenName=my-secret",
		"secretName: db-creds",
		"tokenTtl=3600",
		"authTokenizer=fast",
		"passwordFile=/etc/pw",
		"accessKeyId=AKIAIOSFODNN7EXAMPLE1",
		"monkeyBusiness=high",
		"the auth token is expired",
	}
	runTable(t, func() []struct{ name, in, want string } {
		var out []struct{ name, in, want string }
		for _, s := range same {
			out = append(out, struct{ name, in, want string }{s, s, s})
		}
		return out
	}())
}

func TestRedactURLUserinfoShapes(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"empty user", "redis://:s3cretpass@redis:6379/0",
			"redis://[redacted]@redis:6379/0"},
		{"question mark", "postgres://app:pa?ss@db:5432/x",
			"postgres://[redacted]@db:5432/x"},
		{"hash", "postgres://app:pa#ss@db:5432/x",
			"postgres://[redacted]@db:5432/x"},
		{"both", "amqp://u:a?b#c@mq/vhost", "amqp://[redacted]@mq/vhost"},
		{"plain", "mysql://root:hunter2@db/x", "mysql://[redacted]@db/x"},
	})
}

func TestRedactURLsWithoutUserinfoStayVisible(t *testing.T) {
	same := []string{
		"http://host:8080/x?email=a@b.com",
		"https://example.com/path?q=1#frag",
		"redis://redis:6379/0",
		"http://example.com:8443/docs",
	}
	for _, s := range same {
		if got := Credentials(s); got != s {
			t.Errorf("Credentials(%q) = %q, want it unchanged", s, got)
		}
	}
}

// A key that counts tokens is configuration, not a credential.
func TestRedactKeepsTokenCountSettings(t *testing.T) {
	same := []string{
		"maxTokens=4096", "numTokens: 12", "max_tokens=4096",
		"MAX_TOKENS=4096", "inputTokens=55", "tokenCount: 5",
		`{"maxTokens":4096}`, "GET /x?maxTokens=4096",
		`{\"maxTokens\":4096}`, "authTokens=7",
	}
	var cases []struct{ name, in, want string }
	for _, s := range same {
		cases = append(cases, struct{ name, in, want string }{s, s, s})
	}
	runTable(t, cases)
}

// The same keys with a value that is not a number, and a single token
// key, are still credentials.
func TestRedactKeepsRedactingTokenLikeKeys(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"words", "maxTokens=abc123", "maxTokens=[redacted]"},
		{"plural text", "authTokens=abc1", "authTokens=[redacted]"},
		{"one token", "authToken=1234", "authToken=[redacted]"},
		{"refresh", "refreshToken: x1y2", "refreshToken: [redacted]"},
		{"github list", "GITHUB_TOKENS=ghs1",
			"GITHUB_TOKENS=[redacted]"},
		{"json", `{"authToken":1234}`, `{"authToken":"[redacted]"}`},
	})
}

// Only a token word is a count, and a number too long to be a count is
// still a credential.
func TestRedactCountExemptionIsNarrow(t *testing.T) {
	leaks := []string{
		"apiTokens=123456789012", "default_password=12345678",
		"defaultPassword: 98765432", "input_secret=123456789",
		"DB_PASSWORDS=12345678", `{"authTokens": 1234567890}`,
		"used_token=998877", "context_api_key=424242",
	}
	for _, in := range leaks {
		if got := Credentials(in); got == in {
			t.Errorf("Credentials(%q) leaked the value", in)
		}
	}
	for _, s := range []string{
		"maxTokens=4096", "numTokens=12", "max_tokens=8192",
	} {
		if got := Credentials(s); got != s {
			t.Errorf("Credentials(%q) = %q, want it unchanged", s, got)
		}
	}
}
