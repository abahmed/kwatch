package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func selectorEdit() inventory.Change {
	return inventory.Change{Actor: "bob", Fields: []inventory.FieldChange{
		{Path: "spec.selector", Before: "app=api", After: "app=apii"}}}
}

func endedRouting(to inventory.EntityID) inventory.Change {
	return inventory.Change{Actor: "bob", Fields: []inventory.FieldChange{
		inventory.LinkRemoved(inventory.RoutesTo, to)}}
}

// A Service that stopped routing to the failing workload no longer
// fronts it in the model; the change that records the end of its link
// is named as the nearest, and the selector edit behind it is not told
// a second time.
func TestRecentChangesNameTheEndedLinkOfTheRoot(t *testing.T) {
	r := newRig(t, Config{})
	root := entity(kube.KindDeployment, "api")
	svc := entity(kube.KindService, "payments")
	cm := entity(kube.KindConfigMap, "other")
	for _, id := range []inventory.EntityID{root, svc, cm} {
		r.observe(id)
	}
	r.change(cm, 3*time.Minute, dataEdit("eve"))
	r.change(svc, 2*time.Minute, selectorEdit())
	r.change(svc, 2*time.Minute, endedRouting(root))

	got := RecentChanges(r.model, root, at(10*time.Minute))

	require.Len(t, got, 2)
	words, target, ok := EndedLink(got[0])
	assert.True(t, ok, "the ended link comes first")
	assert.Equal(t, "stopped routing to", words)
	assert.Equal(t, root, target)
	assert.Equal(t, cm, got[1].Entity)
}

// When the Service itself is the failing root, its own edit is named,
// and the ended link it brought about is not repeated.
func TestRecentChangesFoldTheEndedLinkIntoItsEdit(t *testing.T) {
	r := newRig(t, Config{})
	svc := entity(kube.KindService, "payments")
	api := entity(kube.KindDeployment, "api")
	r.observe(svc)
	r.observe(api)
	r.change(svc, 2*time.Minute, selectorEdit())
	r.change(svc, 2*time.Minute, endedRouting(api))

	got := RecentChanges(r.model, svc, at(10*time.Minute))

	require.Len(t, got, 1)
	_, _, ended := got[0].RemovedLink()
	assert.False(t, ended)
	assert.Empty(t, NearChanges(r.model, svc, at(10*time.Minute),
		at(5*time.Minute), &got[0]), "the blamed edit's link is not "+
		"named apart")
}

// A workload whose release stopped naming a Service is told as that,
// next to an unrelated edit, when no edit of the workload is blamed.
func TestRecentChangesNameADroppedCall(t *testing.T) {
	r := newRig(t, Config{})
	root := entity(kube.KindDeployment, "api")
	svc := entity(kube.KindService, "payments")
	r.observe(root)
	r.observe(svc)
	r.change(root, 2*time.Minute, inventory.Change{Actor: "bob",
		Fields: []inventory.FieldChange{
			inventory.LinkRemoved(inventory.Calls, svc)}})

	got := RecentChanges(r.model, root, at(10*time.Minute))

	require.Len(t, got, 1)
	words, target, ok := EndedLink(got[0])
	assert.True(t, ok)
	assert.Equal(t, "no longer calls", words)
	assert.Equal(t, svc, target)
	assert.Equal(t, root, got[0].Entity)
}
