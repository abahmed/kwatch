package delivery

import (
	"strings"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/redact"
)

// maxErrorMessageRunes bounds LastErrorMessage.
const maxErrorMessageRunes = 160

// errorMessage is a short, redacted reason for a failed delivery: the
// fixed reason, then the error text without URLs (see loggedError) and
// with credentials removed.
func errorMessage(reason string, err error) string {
	if err == nil {
		return reason
	}
	text := redact.Evidence(loggedError(err))
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return reason
	}
	text = reason + ": " + text
	if utf8.RuneCountInString(text) <= maxErrorMessageRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxErrorMessageRunes-3]) + "..."
}
