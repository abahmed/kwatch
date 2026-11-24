package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A reason on trial (new in the unreleased version) waits for the digest
// even when critical; another finding of the same incident that is not
// on trial still raises the tier as usual.
func TestTierTrialReasonWaitsForTheDigest(t *testing.T) {
	require.True(t, detection.OnTrial(reasons.NodePodNetworkConflict))
	node := entity(kube.KindNode, "n1")
	pods := []inventory.EntityID{entity(kube.KindPod, "api-1")}
	trial := sig(node, reasons.NodePodNetworkConflict, detection.Critical)
	alone := incidentOf(node, pods, trial)
	raised := incidentOf(node, pods, trial,
		sig(node, reasons.NodeNotReady, detection.Critical))

	assert.Equal(t, Digest, tier(alone))
	assert.NotEqual(t, Digest, tier(raised))
}
