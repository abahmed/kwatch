package sns

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubjectTransliteratesAndCollapsesGaps(t *testing.T) {
	cases := [][2]string{
		{"🔴 сервис «платежи» в shop " +
			"падает — 支付失败 ñandú.",
			"shop - nandu."},
		{"café naïve Æther Ørsted straße",
			"cafe naive AEther Orsted strasse"},
		{"a\tb\r\nc   d", "a b c d"},
		{"it’s “fine” – ok…", `it's "fine" - ok...`},
		{"日本語", ""},
		{"  \u200b🔴 日本 pay", "pay"},
	}
	for _, c := range cases {
		assert.Equal(t, c[1], asciiSubject(c[0]), c[0])
	}
}

func TestSubjectNeverStartsWithSpaceAndStaysShort(t *testing.T) {
	got := asciiSubject("日本 " + strings.Repeat("word ", 40))
	assert.LessOrEqual(t, len(got), maxSubjectBytes)
	assert.NotContains(t, got, "  ")
	assert.Equal(t, strings.TrimSpace(got), got)
	assert.NotEmpty(t, got)
	assert.Equal(t, byte('w'), got[0])
}

// An empty pair of quotes made by dropping what was inside goes away; a
// pair that was empty in the subject itself stays.
func TestSubjectKeepsOriginalEmptyQuotes(t *testing.T) {
	assert.Equal(t, `value is "" here`, asciiSubject(`value is "" here`))
	assert.Equal(t, "said here", asciiSubject(`said “🔴” here`))
	assert.Equal(t, "said here", asciiSubject(`said "日本" here`))
}

// A subject of only ASCII is cut at the byte limit.
func TestSubjectIsCutAtTheByteLimit(t *testing.T) {
	got := asciiSubject(strings.Repeat("abcdefghi ", 20))
	assert.LessOrEqual(t, len(got), maxSubjectBytes)
	assert.NotEmpty(t, got)
}
