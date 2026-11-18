package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// helmModel builds release "payments" in "shop" at revision 12 with
// status, which it got age ago, and a Deployment the release deploys.
func helmModel(
	status string, age time.Duration,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	release := kube.HelmReleaseID("shop", "payments")
	put(m, release, t0, map[string]inventory.Value{
		kube.AttrHelmStatus:       inventory.Text(status),
		kube.AttrHelmRevision:     inventory.Number(12),
		kube.AttrHelmStatusSince:  inventory.Time(t0.Add(-age)),
		kube.AttrHelmGoodRevision: inventory.Number(11),
	})
	put(m, newID(kube.KindDeployment, "shop", "payments-api"), t0,
		map[string]inventory.Value{
			kube.AttrRelease: inventory.Text("payments"),
		})
	put(m, newID(kube.KindDeployment, "shop", "other"), t0,
		map[string]inventory.Value{
			kube.AttrRelease: inventory.Text("payments-old"),
		})
	return m, release
}

func detectHelm(m *inventory.Model, id inventory.EntityID,
) []detection.Finding {
	return NewHelmRelease(0).Detect(testDetectorContext(m, t0),
		entityOf(m, id))
}

func TestHelmReleaseFailedRaises(t *testing.T) {
	m, id := helmModel("failed", 20*time.Minute)
	got := detectHelm(m, id)
	require.Len(t, got, 1)
	f := got[0]
	assert.Equal(t, "HelmReleaseFailed", f.Reason)
	assert.Equal(t, detection.Warning, f.Severity)
	assert.True(t, t0.Add(-20*time.Minute).Equal(f.Since))
	assert.Equal(t, "Helm release has failed to upgrade or roll back "+
		"at revision 12", f.Summary)
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: detection.EvidenceHelmGoodRevision, Value: "11"})
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: detection.EvidenceHelmWorkload,
		Value: "deployment/payments-api"})
	assert.NotContains(t, f.Evidence, detection.Evidence{
		Label: detection.EvidenceHelmWorkload, Value: "deployment/other"})
}

func TestHelmReleaseStuckAfterHold(t *testing.T) {
	m, id := helmModel("pending-upgrade", 25*time.Minute)
	got := detectHelm(m, id)
	require.Len(t, got, 1)
	assert.Equal(t, "HelmReleaseStuck", got[0].Reason)
	assert.Contains(t, got[0].Summary,
		"stuck in pending-upgrade at revision 12 since")
}

func TestHelmReleaseQuietCases(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		age    time.Duration
	}{
		{"deployed", "deployed", time.Hour},
		{"superseded", "superseded", time.Hour},
		{"uninstalled", "uninstalled", time.Hour},
		{"pending shorter than the hold", "pending-install",
			DefaultHelmPendingHold - time.Minute},
		{"uninstalling shorter than the hold", "uninstalling",
			time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, id := helmModel(tc.status, tc.age)
			assert.Empty(t, detectHelm(m, id))
		})
	}
}

func TestHelmReleaseUnsyncedSaysNothing(t *testing.T) {
	m, id := helmModel("failed", time.Hour)
	r := detection.NewRegistry(func(k inventory.Kind) bool {
		return k != kube.KindHelmRelease
	}, NewHelmRelease(0))
	assert.Empty(t, r.Evaluate(m, t0, id).Findings)
}

func TestHelmReleaseFirstRevisionIsInstall(t *testing.T) {
	assert.Equal(t, "install", helmAction(1))
}
