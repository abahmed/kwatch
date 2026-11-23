package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A failure of the root of a live incident is not placed elsewhere.
func TestFailureOfTheRootKeepsItsOwnPlace(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("api"))

	assert.Len(t, r.m.Export(), 1)
}
