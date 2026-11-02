package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A colour code, NUL or carriage return between a key and its value must
// not hide the pair from redaction.
func TestRedactCredentialsStripsControlBeforeRedacting(t *testing.T) {
	rows := []struct{ name, in, secret string }{
		{"colour around key",
			"\x1b[31mpassword\x1b[0m=hunter2secret", "hunter2secret"},
		{"colour token", "\x1b[36mtoken\x1b[0m=abcdef0123456789xyz",
			"abcdef0123456789xyz"},
		{"bearer split by reset",
			"Authorization: Bearer\x1b[0m abcdefghijklmnopqrstuvwxyz012345",
			"abcdefghijklmnopqrstuvwxyz012345"},
		{"nul in key", "DB_PASSWORD\x00=hunter2secret", "hunter2secret"},
		{"osc title", "\x1b]0;title\x07password=hunter2secret",
			"hunter2secret"},
		{"progress bar", "10%\r20%\rpassword=hunter2secret",
			"hunter2secret"},
		{"c1 csi", "password\u009b0m=hunter2secret", "hunter2secret"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			for _, out := range []string{
				Credentials(row.in), Evidence(row.in), Diagnostics(row.in),
			} {
				assert.NotContains(t, out, row.secret)
				assert.NotContains(t, out, "\x1b")
				assert.NotContains(t, out, "\x00")
			}
		})
	}
}

func TestStripControlKeepsTextAfterLastCarriageReturn(t *testing.T) {
	assert.Equal(t, "ERROR boom",
		StripControl("10%\r20%\rERROR boom"))
	assert.Equal(t, "a\nb", StripControl("a\r\nb\r\n")[:3])
	assert.Equal(t, "done", StripControl("done\r   "))
}

func TestStripControlKeepsTabsNewlinesAndUnicode(t *testing.T) {
	in := "a\tb\nc é 日本"
	assert.Equal(t, in, StripControl(in))
}

func TestStripControlRemovesEscapesAndControls(t *testing.T) {
	out := StripControl(
		"\x1b[1;32mok\x1b[0m \x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\" +
			" bell\x07 del\x7f lone\x1b",
	)
	assert.Equal(t, "ok link bell del lone", out)
	assert.False(t, strings.ContainsAny(out, "\x1b\x07\x7f"))
}
