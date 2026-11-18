package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestHelmStepsOfferMarkedRollback(t *testing.T) {
	release := kube.HelmReleaseID("shop", "payments")
	failed := detection.Finding{Entity: release,
		Reason: reasons.HelmReleaseFailed, Evidence: []detection.Evidence{
			{Label: detection.EvidenceHelmGoodRevision, Value: "11"}}}
	sure := &rootcause.CauseRecord{Root: release, Score: 0.9}
	steps := helmSteps(release, sure, []detection.Finding{failed})
	require.Len(t, steps, 3)
	assert.Equal(t, "helm history payments -n shop", steps[0].Command)
	assert.False(t, steps[0].Mutating)
	rollback := steps[2]
	assert.True(t, rollback.Mutating)
	assert.Equal(t, "helm rollback payments 11 -n shop", rollback.Command)
	assert.Contains(t, rollback.Text, "(changes the cluster)")
	assert.Contains(t, rollback.Text, "change it in Git instead")

	failed.Evidence = nil
	assert.Len(t, helmSteps(release, sure, []detection.Finding{failed}), 2,
		"no rollback without a deployed revision to go back to")
}
