package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A word that names a credential hides the word after it, as a flag
// does: "mysqladmin password hunter2".
func TestSafeArgsHideThePositionalWordAfterASecretWord(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"mysqladmin", "password", "hunter2"},
			"mysqladmin password changed"},
		{[]string{"tool", "set", "token", "abc123", "now"},
			"tool set token changed now"},
		{[]string{"redis-cli", "auth", "hunter2"},
			"redis-cli auth changed"},
		{[]string{"vault", "login", "secret", "s3cr3t"},
			"vault login secret changed"},
		{[]string{"chpasswd", "password", "alice:hunter2"},
			"chpasswd password changed"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, safeArgs(c.args), c.args)
	}
}

func TestSafeArgsKeepOrdinaryPositionalWords(t *testing.T) {
	args := []string{"/app", "serve", "worker", "--log-level", "info",
		"run-migrations-add-index", "keyboard", "design", "tokenizer"}
	assert.Equal(t, "/app serve worker --log-level info "+
		"run-migrations-add-index keyboard design tokenizer",
		safeArgs(args))
}

func TestSafeArgsHideALongRandomPositionalWord(t *testing.T) {
	assert.Equal(t, "run changed",
		safeArgs([]string{"run", "9f86d081884c7d659a2feaa0c55ad015"}))
}

// A probe path can carry the secret of a webhook: "/hooks/<token>".
func TestSafePathHidesSegmentsThatLookLikeTokens(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/hooks/9f86d081884c7d659a2feaa0c55ad015",
			"/hooks/changed"},
		{"/v1/ready/Zm9vYmFyMTIzNDU2Nzg5MGFiY2RlZg/ok",
			"/v1/ready/changed/ok"},
		{"/ready?key=abc", "/ready?changed"},
		{"/hooks/9f86d081884c7d659a2feaa0c55ad015?x=1",
			"/hooks/changed?changed"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, safePath(c.path), c.path)
	}
}

func TestSafePathKeepsOrdinaryPaths(t *testing.T) {
	for _, path := range []string{"/", "/healthz", "/api/v1/ready",
		"/readiness-probe-v2-endpoint", "/internal/health/check/2024-01-01",
		"/status/liveness_check", "/actuator/health/readiness"} {
		assert.Equal(t, path, safePath(path), path)
	}
}
