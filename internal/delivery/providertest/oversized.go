package providertest

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/notification"
)

// Oversized is an incident whose note and output are each about n bytes
// of multi-byte text, far over any provider's size limit, so a provider
// that does not cut its text sends a request its service rejects.
func Oversized(n int) notification.Message {
	m := withKey(Announce(), "p-huge")
	line := "сбой *payments* [x](http://e) é " +
		strings.Repeat("y", 40) + "\n"
	body := strings.Repeat(line, n/len(line)+1)
	m.Note = "🔴 payments in shop is crash-looping.\n" + body
	m.Short = "🔴 " + strings.Repeat("payments ", n/9+1)
	m.Doc = nil
	m.Output = strings.Split(strings.TrimSpace(body), "\n")
	return m
}

// AssertWithin fails when text is over limit bytes, is not valid UTF-8,
// or leaves a code fence open.
func AssertWithin(t *testing.T, field, text string, limit int) {
	t.Helper()
	if len(text) > limit {
		t.Fatalf("%s is %d bytes, over the limit of %d",
			field, len(text), limit)
	}
	if !utf8.ValidString(text) {
		t.Fatalf("%s was cut inside a character", field)
	}
	if text == "" {
		t.Fatalf("%s is empty", field)
	}
	AssertFencesBalanced(t, text)
}
