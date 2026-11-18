package announce

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A risk a digest named is named once, not again after a restart: the
// digest state the startup marker keeps remembers it.
func TestDigestRiskIsNotNamedAgainAfterRestart(t *testing.T) {
	risk := detection.Finding{
		Entity: inventory.CoreID(kube.KindDeployment, "shop", "web"),
		Reason: reasons.WorkloadNeverReady, Advisory: true}
	risks := func() []detection.Finding {
		return []detection.Finding{risk}
	}
	c, _ := foldHarness(demotedRecord("a"))
	c.SetAdvisories(risks)
	c.Low.Since = foldNow.Add(-DigestWindow)
	require.Len(t, c.PendingRisks(), 1)
	c.CollectDigest(context.Background(), foldNow, nil)
	require.Empty(t, c.PendingRisks(), "named by the digest")

	again, _ := foldHarness(demotedRecord("a"))
	again.SetAdvisories(risks)
	again.Startup.Summary = c.Startup.Summary.Clone()
	again.RestoreDigest()

	assert.Empty(t, again.PendingRisks(), "a restart names it again")
}
