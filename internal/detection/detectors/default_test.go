package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDetectorsAreDistinct(t *testing.T) {
	all := Default()
	assert.Len(t, all, 35)
	names := map[string]bool{}
	for _, d := range all {
		assert.False(t, names[d.Name()], "duplicate detector %s", d.Name())
		names[d.Name()] = true
		assert.NotEmpty(t, d.Kinds(), d.Name())
	}
	assert.Equal(t, "container", all[0].Name(),
		"the order is the registry's evaluation order")
}
