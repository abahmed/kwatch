package redact

import (
	"strings"
	"testing"
)

// A redaction must replace the secret and nothing next to it: the text
// around it is what the reader needs to understand the line.
func TestRedactionLeavesNeighbouringTextAlone(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"closing quote of a quoted header",
			`curl -H 'X-Api-Key: abcd' https://shop.test`,
			`curl -H 'X-Api-Key: [redacted]' https://shop.test`},
		{"closing double quote",
			`run "password=abcd" next`,
			`run "password=[redacted]" next`},
		{"parameters after the secret in a query string",
			`jdbc:mysql://db:3306/shop?user=app&password=hunter2` +
				`&useSSL=false`,
			`jdbc:mysql://db:3306/shop?user=app&password=[redacted]` +
				`&useSSL=false`},
		{"parameters after the secret in a form body",
			`user=app&password=hunter2&next=x`,
			`user=app&password=[redacted]&next=x`},
		{"the next pair of a connection string",
			`Server=db;Password=hunter2;User=x`,
			`Server=db;Password=[redacted];User=x`},
		{"a progress counter is not a password",
			`pass=3/5 checks`, `pass=3/5 checks`},
		{"a small count is not a password",
			`pass=12 fail=0`, `pass=12 fail=0`},
	}
	for _, tc := range cases {
		if got := Credentials(tc.in); got != tc.want {
			t.Errorf("%s:\n got  %s\n want %s", tc.name, got, tc.want)
		}
	}
}

// The narrower rules must not let a secret through.
func TestRedactionStillHidesSecretsNextToTheNewBoundaries(t *testing.T) {
	cases := []struct{ name, in, secret string }{
		{"an ampersand inside a password", `password=a&b9Xk x`, "b9Xk"},
		{"a quote inside a password", `password=a'b9Xk x`, "b9Xk"},
		{"a numeric password", `pass=1234 x`, "1234"},
		{"a word password", `pass=hunter2 x`, "hunter2"},
		{"a counter-shaped value of another key",
			`password=3/5 x`, "3/5"},
	}
	for _, tc := range cases {
		if got := Credentials(tc.in); strings.Contains(got, tc.secret) {
			t.Errorf("%s: %q leaked in %q", tc.name, tc.secret, got)
		}
	}
}

// A raw password may hold "&", ";" and quotes. Outside a query string
// or a connection string they are part of it: the value is redacted to
// the end of its word, not cut where its tail looks like a next pair.
func TestRedactionHidesTheTailOfARawPassword(t *testing.T) {
	cases := []struct{ in, secret string }{
		{`password=p&ss=word1`, "word1"},
		{`api_key=AAA;'BBB`, "BBB"},
		{`password=ab;c=secretpart`, "secretpart"},
		{`DB_PASSWORD=p&ss=w0rd next`, "w0rd"},
		{`env: secret=x,y=tail2 next`, "tail2"},
	}
	for _, tc := range cases {
		got := Credentials(tc.in)
		if strings.Contains(got, tc.secret) {
			t.Errorf("%q leaked in %q", tc.secret, got)
		}
		if !strings.Contains(got, redacted) {
			t.Errorf("%q not redacted: %q", tc.in, got)
		}
	}
}
