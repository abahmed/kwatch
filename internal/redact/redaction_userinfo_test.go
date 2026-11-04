package redact

import "testing"

// A user:password@host written without a scheme still leaks the password.
func TestRedactBareUserinfoRedactsPassword(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"dsn in prose", "connecting to admin:s3cret@db:5432 failed",
			"connecting to admin:[redacted]@db:5432 failed"},
		{"start of line", "admin:s3cret@db.svc.local",
			"admin:[redacted]@db.svc.local"},
		{"quoted log line", `msg="dial admin:s3cret@db:5432: refused"`,
			`msg="dial admin:[redacted]@db:5432: refused"`},
		{"after equals", "dsn=app_user:pa.ss-w0rd@pg-0",
			"dsn=app_user:[redacted]@pg-0"},
		{"at in password", "login bob:p@ss@db failed",
			"login bob:[redacted]@db failed"},
		{"ipv6 host", "dial bob:hunter2@[fd00::1]:5432",
			"dial bob:[redacted]@[fd00::1]:5432"},
		{"in parens", "(bob:hunter2@cache)", "(bob:[redacted]@cache)"},
	})
}

// Shapes with a colon and an "@" that hold no password stay as they are.
func TestRedactBareUserinfoKeepsLookalikes(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"email", "sent to alice@example.com", "sent to alice@example.com"},
		{"host port", "dial db:5432 refused", "dial db:5432 refused"},
		{"ipv6", "listen [fe80::1]:8080", "listen [fe80::1]:8080"},
		{"time", "at 12:30:45@node-1", "at 12:30:45@node-1"},
		{"short time", "at 12:30@node-1", "at 12:30@node-1"},
		{"mention", "cc @alice and @bob: done", "cc @alice and @bob: done"},
		{"duration then mention", "after 2m0s:*@channel.",
			"after 2m0s:*@channel."},
		{"chat mention", "ping:all@here", "ping:all@here"},
		{"mailto", "mailto:bob@example.com", "mailto:bob@example.com"},
		{"from key", "from:ops@example.com", "from:ops@example.com"},
		{"image digest", "image nginx:1.25@sha256:abcdef0123",
			"image nginx:1.25@sha256:abcdef0123"},
		{"image path digest", "registry.io/org/app:v1@sha256:abcd",
			"registry.io/org/app:v1@sha256:abcd"},
		{"url handled once", "postgres://admin:s3cret@db/app",
			"postgres://[redacted]@db/app"},
		{"already redacted", "admin:[redacted]@db",
			"admin:[redacted]@db"},
		{"ssh remote", "git@github.com:org/repo.git",
			"git@github.com:org/repo.git"},
	})
}

// A password may hold URL and shell punctuation, ":" and "@", and the
// "user:" may follow a backtick, a brace or a key's colon.
func TestRedactBareUserinfoRedactsPunctuatedPasswords(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"hash", "connecting to admin:P#ssw0rd@db:5432",
			"connecting to admin:[redacted]@db:5432"},
		{"slash", "connecting to admin:pa/ss@db:5432",
			"connecting to admin:[redacted]@db:5432"},
		{"query and semicolon", "dial admin:p?s;s@db",
			"dial admin:[redacted]@db"},
		{"caret and braces", "dial admin:p^{s}s@db",
			"dial admin:[redacted]@db"},
		{"colon", "connecting to admin:pa:ss@db:5432",
			"connecting to admin:[redacted]@db:5432"},
		{"colon and at", "dial admin:p@ss:w0rd@db.prod:5432",
			"dial admin:[redacted]@db.prod:5432"},
		{"backtick", "connecting to `admin:s3cret@db`",
			"connecting to `admin:[redacted]@db`"},
		{"after a key colon", "dsn:admin:s3cret@db:5432",
			"dsn:[redacted]@db:5432"},
		{"in braces", "dsn={admin:s3cret@db.prod:5432/app}",
			"dsn={admin:[redacted]@db.prod:5432/app}"},
		{"address key then password", "email:admin:pw@db",
			"email:[redacted]@db"},
		{"ipv4 host", "failed admin:s3cret@10.0.0.5:5432",
			"failed admin:[redacted]@10.0.0.5:5432"},
	})
}

// A package version, the rest of a URL, a mail subject's reply prefix
// and a plain name after a "…name" key are not userinfo. The last two
// are kept on purpose: a letters-only "password" after a "…name" key
// is a node name ("rabbit@host"), and anything else is redacted.
func TestRedactBareUserinfoKeepsVersionsAndNames(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"npm version", " npm:lodash@4.17.21", " npm:lodash@4.17.21"},
		{"npm alias", "npm i alias:npm:lodash@4.17.21-beta.1",
			"npm i alias:npm:lodash@4.17.21-beta.1"},
		{"url tail", "http://example.com:8080/path?x=1@y",
			"http://example.com:8080/path?x=1@y"},
		{"purl", "pkg:npm/lodash@4.17.21", "pkg:npm/lodash@4.17.21"},
		{"reply subject", "Subject: re:bob@example.com",
			"Subject: re:bob@example.com"},
		{"rabbit node", "cluster_name:rabbit@rabbitmq-0",
			"cluster_name:rabbit@rabbitmq-0"},
		{"name key with a real password", "username:s3cret@db",
			"username:[redacted]@db"},
	})
}
