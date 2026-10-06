package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func skippedOf(kinds ...string) []plannedResource {
	out := make([]plannedResource, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, plannedResource{kind: kind})
	}
	return out
}

func TestDynamicSourceLogsSkippedKindsOnlyWhenTheyChange(t *testing.T) {
	src := &DynamicSource{}

	kinds, first := src.skippedChanged(skippedOf("TraefikService", "Alpha"))
	_, again := src.skippedChanged(skippedOf("Alpha", "TraefikService"))
	_, grown := src.skippedChanged(skippedOf("Alpha", "Beta"))

	assert.True(t, first, "the first pass logs the list")
	assert.Equal(t, "Alpha,TraefikService", kinds)
	assert.False(t, again, "the same list is not logged again")
	assert.True(t, grown)
}

func TestDynamicSourceDoesNotLogAnEmptySkippedList(t *testing.T) {
	src := &DynamicSource{}
	src.skippedChanged(skippedOf("Alpha"))

	_, emptied := src.skippedChanged(nil)
	_, back := src.skippedChanged(skippedOf("Alpha"))

	assert.False(t, emptied)
	assert.True(t, back, "a list that returns after being empty is news")
}
