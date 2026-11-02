package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Colour codes, NULs and progress-bar carriage returns in a log must not
// let a secret past redaction, and no escape reaches an excerpt.
func TestLogLinesStripsControlBeforeRedacting(t *testing.T) {
	body := "\x1b[31mpassword\x1b[0m=hunter2secret\n" +
		"Authorization: Bearer\x1b[0m abcdefghijklmnopqrstuvwxyz012345\n" +
		"DB_PASSWORD\x00=hunter2secret\n" +
		"10%\r20%\rERROR boom\n"

	lines := logLines([]byte(body))

	joined := strings.Join(lines, "\n")
	assert.NotContains(t, joined, "hunter2secret")
	assert.NotContains(t, joined, "abcdefghijklmnopqrstuvwxyz012345")
	assert.NotContains(t, joined, "\x1b")
	assert.NotContains(t, joined, "\x00")
	assert.Contains(t, lines, "ERROR boom")
}

func TestEventAndTerminationTextStripControlBeforeRedacting(t *testing.T) {
	in := "\x1b[36mtoken\x1b[0m=abcdef0123456789xyz"
	for _, got := range []string{evidenceText(in), eventText(in)} {
		assert.NotContains(t, got, "abcdef0123456789xyz")
		assert.NotContains(t, got, "\x1b")
	}
}
