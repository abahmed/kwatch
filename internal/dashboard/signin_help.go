package dashboard

import (
	"bytes"
	"html"
	"strings"
)

// signInHelpMark is where the sign-in page shows Options.SignInHelp.
const signInHelpMark = "{{signin-help}}"

// withSignInHelp returns the assets with the sign-in help text in the
// shell, above the link to the docs on getting the password. The text
// is escaped, so it is shown as written and never read as HTML; a line
// break stays one. Empty text leaves no trace. The assets are copied:
// each server has its own.
func (a *assets) withSignInHelp(text string) *assets {
	out := *a
	out.shell = bytes.Replace(a.shell, []byte(signInHelpMark),
		[]byte(signInHelpHTML(text)), 1)
	return &out
}

func signInHelpHTML(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = html.EscapeString(strings.TrimSpace(line))
	}
	return `<p class="help" data-signin-help>` +
		strings.Join(lines, "<br>") + `</p>`
}
