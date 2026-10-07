package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func setOwner(t *testing.T, r *rig, id inventory.EntityID, owner string) {
	t.Helper()
	observeAttrs(t, r, id, map[string]inventory.Value{
		kube.AttrOwner: inventory.Text(owner)})
}

func namespaceID() inventory.EntityID {
	return inventory.CoreID(kube.KindNamespace, "", "shop")
}

func announceOwner(t *testing.T, r *rig) Decision {
	t.Helper()
	r.raise(at(0), podSig("web"))
	ds := r.tick(at(DefaultSettle))
	wantAction(t, ds, Announce, "settled")
	return ds[0]
}

func TestOwnerOfTheRootRoutesTheIncident(t *testing.T) {
	r := newRig(t, Config{})
	setOwner(t, r, entity(kube.KindPod, "web"), "payments")

	d := announceOwner(t, r)

	assert.Equal(t, "payments", d.Incident.Owner)
	assert.Equal(t, []string{"payments"}, d.Incident.AnnouncedRoute.Owners)
}

func TestOwnerIsInheritedFromTheOwningWorkload(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	deploy := entity(kube.KindDeployment, "web")
	r.relate(pod, inventory.OwnedBy, deploy)
	setOwner(t, r, deploy, "search")

	d := announceOwner(t, r)

	assert.Equal(t, "search", d.Incident.Owner)
}

func TestOwnerFallsBackToTheNamespace(t *testing.T) {
	r := newRig(t, Config{})
	setOwner(t, r, namespaceID(), "platform")

	d := announceOwner(t, r)

	assert.Equal(t, "platform", d.Incident.Owner)
}

func TestWorkloadOwnerBeatsNamespaceOwner(t *testing.T) {
	r := newRig(t, Config{})
	setOwner(t, r, namespaceID(), "platform")
	setOwner(t, r, entity(kube.KindPod, "web"), "payments")

	d := announceOwner(t, r)

	assert.Equal(t, "payments", d.Incident.Owner)
}

func TestNoOwnerAnywhereMeansTheDefaultRoute(t *testing.T) {
	r := newRig(t, Config{})

	d := announceOwner(t, r)

	assert.Empty(t, d.Incident.Owner)
	assert.Empty(t, d.Incident.AnnouncedRoute.Owners)
}

func TestOwnerEditedAfterAnnouncementShowsOnTheNextUpdate(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	setOwner(t, r, pod, "payments")
	announceOwner(t, r)

	setOwner(t, r, pod, "search")
	ds := r.tick(at(DefaultSettle + RemindEvery))

	wantAction(t, ds, Update, ReasonReminder)
	assert.Equal(t, "search", ds[0].Incident.Owner)
	assert.Equal(t, []string{"payments", "search"},
		ds[0].Incident.AnnouncedRoute.Owners,
		"the conversation stays routable to who was told")
}
