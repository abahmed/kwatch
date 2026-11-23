package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func stuckMember(id inventory.EntityID) detection.Finding {
	return detection.Finding{Entity: id, Reason: "Terminating",
		Mode: detection.ModeTerminating}
}

// An incident whose root recovered, with only deletions left, is
// draining: the recovery is no news.
func TestDrainingIncidentHasOnlyStuckDeletionsBesideItsRoot(t *testing.T) {
	ns := inventory.CoreID(kube.KindNamespace, "", "old-shop")
	api := inventory.CoreID(kube.KindAPIService, "", "v1beta1.x.io")
	member := stuckMember(ns)
	p := &Incident{Root: api, Members: map[detection.Key]detection.Finding{
		member.Key(): member}}

	assert.True(t, draining(p))

	own := stuckMember(api)
	p.Members[own.Key()] = own
	assert.False(t, draining(p), "the root still fails")
}
