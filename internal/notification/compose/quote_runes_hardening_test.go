package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A control rune a pod hides inside a word must not split the word past
// the redactor: "pass\x1eword=x" reads "password=x" once the rune is gone,
// so the rune goes first and the redactor sees the word.
func TestClipDropsQuoteRunesBeforeRedacting(t *testing.T) {
	for _, text := range []string{
		"login failed pass\x1eword=hunter2xyz",
		"login failed pass\x1fword=hunter2xyz",
		"login failed pa\x1es\x1fsword: hunter2xyz",
		"login failed to\x1eken=abcdef123456 next",
	} {
		got := clip(text)

		assert.NotContains(t, got, "hunter2xyz", text)
		assert.NotContains(t, got, "abcdef123456", text)
		assert.False(t, strings.ContainsAny(got, quoteOpen+quoteClose), text)
	}
}

// Every plain part and every payload of a split text is free of stray
// quote runes, whichever side of a quotation they sit on.
func TestSegmentsOfScrubsStrayQuoteRunes(t *testing.T) {
	text := "a\x1f b \x1e x\x1ey \x1f mid \x1e one \x1e two \x1f " +
		"tail\x1f end \x1e open"

	for _, seg := range segmentsOf(text) {
		assert.False(t, strings.ContainsAny(seg.text, quoteOpen+quoteClose),
			"%q holds a quote rune", seg.text)
	}
}

func TestSegmentsOfKeepsTheBytesBetweenTheRunes(t *testing.T) {
	text := "see " + markQuote(`say "hi"`) + " now"

	got := segmentsOf(text)

	assert.Equal(t, []quoteSegment{{text: "see "},
		{text: `say "hi"`, quoted: true}, {text: " now"}}, got)
}
