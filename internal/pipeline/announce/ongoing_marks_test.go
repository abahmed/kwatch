package announce

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A problem seen 5 times at the last digest, whose notes then aged out
// to 1 and grew to 2, came back: the stale mark of 5 must not hide it.
func TestOngoingMarkFollowsTheCountDown(t *testing.T) {
	c := &Collector{ongoingMarks: map[string]int{"a": 5}}
	assert.False(t, c.cameBack("a", 1), "fewer notes is not a return")
	assert.True(t, c.cameBack("a", 2), "a rise above the lowered mark is")
}

func TestOngoingFirstLookOnlyRemembers(t *testing.T) {
	c := &Collector{ongoingMarks: map[string]int{}}
	assert.False(t, c.cameBack("a", 3))
	assert.Equal(t, 3, c.ongoingMarks["a"])
}
