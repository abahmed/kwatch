package notification

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A "<" far from the cut is text, not a tag cut in half: the message
// keeps everything up to the cut instead of ending at the "<".
func TestTruncateKeepsTextAfterALoneLessThan(t *testing.T) {
	text := "x<y " + strings.Repeat("a", 5000)

	got := Truncate(text, 100)

	assert.Len(t, got, 100)
	assert.True(t, strings.HasPrefix(got, "x<y aaaa"), got)
}

// A tag cut in half near the cut is still dropped, a link included.
func TestTruncateStillDropsATagCutInHalf(t *testing.T) {
	link := `<a href="https://example.test/` + strings.Repeat("p", 120)
	got := TruncateHTML("see "+link+`">here</a>`, 100)

	assert.Equal(t, "see …", got)
}

// Plain text that happens to hold "<a b=" is not markup: its tail stays.
func TestTruncateKeepsPlainTextThatLooksLikeATag(t *testing.T) {
	text := `error <a href="x` + strings.Repeat("p", 300)

	got := Truncate(text, 200)

	assert.Len(t, got, 200)
	assert.True(t, strings.HasPrefix(got, `error <a href="xppp`), got)
}

// No room means nothing is sent, never the whole text.
func TestTruncateWithoutRoomIsEmpty(t *testing.T) {
	long := strings.Repeat("a", 5000)

	assert.Empty(t, Truncate(long, 0))
	assert.Empty(t, Truncate(long, -3))
	assert.Empty(t, TruncateHTML(long, 0))
}

// A cut that falls inside a word ends before it, so a quote never
// stops halfway through a word.
func TestTruncateEndsOnAWholeWord(t *testing.T) {
	text := "the merchant key is not valid for this account"

	got := Truncate(text, 24)

	assert.Equal(t, "the merchant key is…", got)
}

// A text with no space near the limit is still cut at the limit.
func TestTruncateWithoutANearbySpaceCutsAtTheLimit(t *testing.T) {
	text := "a " + strings.Repeat("b", 200)

	got := Truncate(text, 100)

	assert.Len(t, got, 100)
}
