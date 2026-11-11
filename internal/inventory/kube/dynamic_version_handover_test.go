package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// A type whose served version changes hands its watch over to the new
// version: nothing is retired, so live objects are not reported gone.
func TestDynamicSourceHandsAVersionChangeOver(t *testing.T) {
	d := NewDynamicSource(DynamicConfig{})
	v1 := widgetGVR
	v2 := schema.GroupVersionResource{Group: v1.Group, Version: "v2",
		Resource: v1.Resource}
	cancelled := false
	old := &dynamicWatch{kind: "widget",
		cancel: func() { cancelled = true }}
	d.running[v1] = old
	desired := []plannedResource{{gvr: v2, kind: "widget"}}

	d.handOverVersions(desired)
	retired := d.dropUnwanted(desired, nil, true)

	assert.True(t, cancelled, "the old version's informer stops")
	assert.Same(t, old, d.parked[v2], "the new version takes it over")
	assert.Empty(t, retired, "nothing is reported gone")
	assert.Nil(t, d.running[v1])
}
