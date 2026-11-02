package dashboard

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// With more texts than one generation holds, a second read of all of
// them redacts again only what fell out of both generations, not
// nearly everything (the old cache emptied itself when full), and the
// cache never holds more than two generations.
func TestTextCacheKeepsTextsInUseWhenAGenerationFills(t *testing.T) {
	misses := 0
	c := textCache{fn: func(s string) string {
		misses++
		return "r:" + s
	}}
	const extra = 1000
	n := maxCachedTexts + extra
	for i := range n {
		c.redact("t" + strconv.Itoa(i))
	}
	misses = 0
	for i := range n {
		s := "t" + strconv.Itoa(i)
		assert.Equal(t, "r:"+s, c.redact(s))
	}

	assert.LessOrEqual(t, misses, extra)
	assert.LessOrEqual(t, len(c.cur)+len(c.prev), 2*maxCachedTexts)
}
