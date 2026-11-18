package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Ownership follows relations, never names: the autoscaler that
// scales web is web's whatever it is called, and a pod named like web
// but owned by web-admin is web-admin's.
func TestBelongsFollowsRelations(t *testing.T) {
	r := newRig(t, Config{})
	web := entity(kube.KindDeployment, "web")
	admin := entity(kube.KindDeployment, "web-admin")
	hpa := entity(kube.KindHPA, "frontend")
	pod := entity(kube.KindPod, "web-admin-5c-x")
	box := entity(kube.KindContainer, "web-admin-5c-x/app")
	r.relate(hpa, inventory.Scales, web)
	r.relate(pod, inventory.OwnedBy, admin)
	r.relate(box, inventory.PartOf, pod)
	p := &Incident{Root: web, Members: map[detection.Key]detection.Finding{
		{Entity: hpa}: {Entity: hpa}, {Entity: box}: {Entity: box}}}

	b := belongsOf(r.model, p)

	owned, known := b.Of(web, hpa)
	assert.True(t, owned && known, "its own autoscaler")
	owned, known = b.Of(web, box)
	assert.True(t, known)
	assert.False(t, owned, "named like web, owned by web-admin")
	owned, _ = b.Of(admin, box)
	assert.True(t, owned)
	_, known = b.Of(web, entity(kube.KindService, "web"))
	assert.False(t, known, "not an object of the incident")
}
