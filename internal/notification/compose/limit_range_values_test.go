package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A LimitRange change shows the bounds that changed, not the empty ones.
func TestLimitRangeChangeShowsOnlyTheBoundsThatChanged(t *testing.T) {
	change := inventory.Change{
		Entity: inventory.CoreID(kube.KindLimitRange, "billing", "bounds"),
		Fields: []inventory.FieldChange{{Path: "spec.limits",
			Before: "Container min ; max cpu=4; default ; " +
				"defaultRequest ; ratio ",
			After: "Container min ; max cpu=1; default ; " +
				"defaultRequest ; ratio "}}}

	got := changeValues(change)

	assert.Equal(t, " from Container max cpu=4 to Container max cpu=1",
		got)
}

func TestLimitRangeChangeSaysWhenABoundIsAddedOrDropped(t *testing.T) {
	before, after := limitRangeValues(
		"Container min cpu=100m; max cpu=1; default ; defaultRequest ; "+
			"ratio ",
		"Container min cpu=100m; max cpu=1; default cpu=500m; "+
			"defaultRequest ; ratio ")

	assert.Equal(t, "nothing", before)
	assert.Equal(t, "Container default cpu=500m", after)
}
