package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A same-named object of another kind is not the workload's own
// without a relation; with one, its own autoscaler is.
func TestOwnedNameFollowsRelations(t *testing.T) {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "web-settings")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-7d-a")
	p := incident.Incident{Root: web}
	assert.False(t, ownedName(p, web, hpa))
	assert.False(t, ownedName(p, web, cm))
	assert.True(t, ownedName(p, web, pod))
	p.Belongs = incident.Belongs{hpa: web}
	assert.True(t, ownedName(p, web, hpa))
	p.Members = map[detection.Key]detection.Finding{
		{Entity: cm}: {Entity: cm}}
	assert.False(t, failsOnItsOwn(p, web),
		"a ConfigMap named after web is not web failing on its own")
}
