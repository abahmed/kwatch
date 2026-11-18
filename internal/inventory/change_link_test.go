package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRemovedLinkReadsBackWhatLinkRemovedWrote(t *testing.T) {
	to := NewEntityID("gateway.networking.k8s.io", "httproute", "shop",
		"web")
	c := Change{Fields: []FieldChange{LinkRemoved(RoutesTo, to)}}
	rel, target, ok := c.RemovedLink()
	assert.True(t, ok)
	assert.Equal(t, RoutesTo, rel)
	assert.Equal(t, to, target)

	for _, other := range []Change{
		{Fields: []FieldChange{{Path: "spec.selector"}}},
		{Fields: []FieldChange{{Path: "link.selects", Before: "x"}}},
		{Created: true, Fields: c.Fields},
		{Fields: append(c.Fields, FieldChange{Path: "spec"})},
	} {
		_, _, ok := other.RemovedLink()
		assert.False(t, ok, "%+v", other)
	}
}

// An edit and the end of a link it brought about, at the same time on
// the same object, are two changes.
func TestSameChangeTellsAnEndedLinkFromItsEdit(t *testing.T) {
	at := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	id := CoreID("service", "shop", "payments")
	edit := Change{Entity: id, At: at,
		Fields: []FieldChange{{Path: "spec.selector"}}}
	ended := Change{Entity: id, At: at, Fields: []FieldChange{
		LinkRemoved(RoutesTo, CoreID("deployment", "shop", "api"))}}
	other := Change{Entity: id, At: at, Fields: []FieldChange{
		LinkRemoved(RoutesTo, CoreID("deployment", "shop", "web"))}}
	assert.True(t, SameChange(edit, edit))
	assert.True(t, SameChange(ended, ended))
	assert.False(t, SameChange(edit, ended))
	assert.False(t, SameChange(ended, other))
}
