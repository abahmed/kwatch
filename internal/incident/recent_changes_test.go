package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func dataEdit(actor string) inventory.Change {
	return inventory.Change{Actor: actor, Fields: []inventory.FieldChange{
		{Path: "data.LOG", Before: "a", After: "b"}}}
}

// Recent changes are the few edits worth naming, newest first; noise
// such as leases, status writes and autoscaler replicas is left out.
func TestRecentChangesKeepsOnlyWhatIsWorthNaming(t *testing.T) {
	r := newRig(t, Config{})
	root := entity(kube.KindDeployment, "api")
	cm := entity(kube.KindConfigMap, "app-config")
	other := inventory.CoreID(kube.KindConfigMap, "billing", "elsewhere")
	lease := entity(kube.KindLease, "kwatch")
	web := entity(kube.KindDeployment, "web")
	for _, id := range []inventory.EntityID{root, cm, other, lease, web} {
		r.observe(id)
	}
	r.change(cm, 2*time.Minute, dataEdit("bob"))
	r.change(other, 3*time.Minute, dataEdit("eve"))
	r.change(lease, 4*time.Minute, dataEdit("kwatch"))
	r.change(web, 5*time.Minute, inventory.Change{
		Fields: []inventory.FieldChange{{Path: "spec.replicas"}}})
	r.change(root, 6*time.Minute, inventory.Change{
		Actor: "hpa", Fields: nil})
	r.change(root, 8*time.Minute, image("api:1", "api:2"))

	got := RecentChanges(r.model, root, at(10*time.Minute))

	assert.Len(t, got, 2)
	assert.Equal(t, root, got[0].Entity, "newest first")
	assert.Equal(t, cm, got[1].Entity)
	assert.Equal(t, "bob", got[1].Actor)
}

// Only the last 30 minutes count, and at most three are named.
func TestRecentChangesIsBoundedInTimeAndCount(t *testing.T) {
	r := newRig(t, Config{})
	root := entity(kube.KindDeployment, "api")
	r.observe(root)
	names := []string{"a", "b", "c", "d"}
	for i, name := range names {
		cm := entity(kube.KindConfigMap, name)
		r.observe(cm)
		r.change(cm, time.Duration(40+i)*time.Minute, dataEdit("bob"))
	}
	old := entity(kube.KindConfigMap, "old")
	r.observe(old)
	r.change(old, time.Minute, dataEdit("bob"))

	got := RecentChanges(r.model, root, at(45*time.Minute))

	assert.Len(t, got, MaxRecentChanges)
	assert.Equal(t, "d", got[0].Entity.Name)
	assert.Nil(t, RecentChanges(nil, root, at(time.Hour)))
}
