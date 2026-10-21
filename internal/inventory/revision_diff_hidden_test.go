package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A value that may not be shown reads "changed" on both sides, yet the
// edit happened: dropping it hides the only change of a revision.
func TestRevisionDiffKeepsAnEditWhoseValuesAreHidden(t *testing.T) {
	hidden := edit("containers[x].env.FOO", HiddenValue, HiddenValue)

	got := RevisionDiff([]Change{{Fields: []FieldChange{hidden}}})

	assert.Equal(t, []FieldChange{hidden}, got)
}

func TestRevisionDiffKeepsAHiddenEditFoldedWithAnother(t *testing.T) {
	got := RevisionDiff([]Change{
		{Fields: []FieldChange{
			edit("containers[x].env.FOO", HiddenValue, HiddenValue)}},
		{Fields: []FieldChange{
			edit("containers[x].env.FOO", HiddenValue, HiddenValue)}},
	})

	assert.Len(t, got, 1)
}
