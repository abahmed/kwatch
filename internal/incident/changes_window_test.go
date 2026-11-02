package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The dashboard lists what changed on the root and what it uses inside
// a window; a neighbour in the namespace is not part of the incident.
func TestChangesBetweenKeepsNearChangesInWindow(t *testing.T) {
	r := newRig(t, Config{})
	root := entity(kube.KindDeployment, "api")
	cm := entity(kube.KindConfigMap, "app-config")
	neighbour := entity(kube.KindConfigMap, "other")
	for _, id := range []inventory.EntityID{root, cm, neighbour} {
		r.observe(id)
	}
	r.relate(root, inventory.References, cm)
	r.change(cm, 2*time.Minute, dataEdit("bob"))
	r.change(neighbour, 3*time.Minute, dataEdit("eve"))
	r.change(root, 8*time.Minute, image("api:1", "api:2"))
	r.change(root, 30*time.Minute, image("api:2", "api:3"))

	got := ChangesBetween(r.model, root, at(time.Minute),
		at(10*time.Minute), 5)

	assert.Len(t, got, 2)
	assert.Equal(t, root, got[0].Entity, "the root leads")
	assert.Equal(t, at(8*time.Minute), got[0].At)
	assert.Equal(t, cm, got[1].Entity)
}

func TestChangesBetweenIsBoundedAndSafeWithoutModel(t *testing.T) {
	r := newRig(t, Config{})
	root := entity(kube.KindDeployment, "api")
	r.observe(root)
	var used []inventory.EntityID
	for i, name := range []string{"a", "b", "c"} {
		cm := entity(kube.KindConfigMap, name)
		r.observe(cm)
		used = append(used, cm)
		r.change(cm, time.Duration(i+1)*time.Minute, dataEdit("bob"))
	}
	// One observation carries every reference: a later one replaces it.
	r.relate(root, inventory.References, used...)

	got := ChangesBetween(r.model, root, t0, at(time.Hour), 2)

	assert.Len(t, got, 2)
	assert.Equal(t, "c", got[0].Entity.Name, "newest first")
	assert.Empty(t, ChangesBetween(r.model, root, t0, at(time.Hour), 0))
	assert.Empty(t, ChangesBetween(nil, root, t0, at(time.Hour), 2))
}
