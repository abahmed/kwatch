package providertest

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/notification"
)

// AssertOneLeadingEmoji fails unless text starts with exactly one status
// marker and carries no other emoji.
func AssertOneLeadingEmoji(t *testing.T, text string) {
	t.Helper()
	text = strings.TrimSpace(text)
	marker := ""
	for _, candidate := range notification.Markers() {
		if strings.HasPrefix(text, candidate) {
			marker = candidate
		}
	}
	if marker == "" {
		t.Fatalf("text does not start with a status marker: %q", text)
	}
	if n := CountEmoji(text); n != 1 {
		t.Fatalf("text has %d emoji, want exactly 1: %q", n, text)
	}
}

// CountEmoji counts pictographic runes (status markers and friends).
func CountEmoji(text string) int {
	count := 0
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if isEmoji(r) {
			count++
		}
		text = text[size:]
	}
	return count
}

func isEmoji(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF)
}
