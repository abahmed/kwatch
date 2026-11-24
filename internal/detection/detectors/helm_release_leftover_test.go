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

// An uninstall stuck for days that left none of the release's
// workloads is Low: it is the record of a removed installation. One
// that still has a workload, or is younger than a day, keeps Warning.
func TestHelmUninstallStuckLongWithNothingLeftIsLow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		age      time.Duration
		workload bool
		want     detection.Severity
	}{
		{"nothing left", 3 * DefaultLeftoverAge, false, detection.Info},
		{"a workload left", 3 * DefaultLeftoverAge, true,
			detection.Warning},
		{"young", DefaultLeftoverAge / 2, false, detection.Warning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, id := releaseOnly("uninstalling", tc.age, tc.workload)
			got := detectHelm(m, id)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].Severity)
		})
	}
}

// A stuck upgrade is news however old it is.
func TestHelmPendingUpgradeStuckLongStaysWarning(t *testing.T) {
	m, id := releaseOnly("pending-upgrade", 3*DefaultLeftoverAge, false)
	got := detectHelm(m, id)
	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

// releaseOnly is release "payments" in "shop" with status, which it got
// age ago, and its Deployment when workload is set.
func releaseOnly(
	status string, age time.Duration, workload bool,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	release := kube.HelmReleaseID("shop", "payments")
	put(m, release, t0, map[string]inventory.Value{
		kube.AttrHelmStatus:      inventory.Text(status),
		kube.AttrHelmRevision:    inventory.Number(12),
		kube.AttrHelmStatusSince: inventory.Time(t0.Add(-age)),
	})
	if workload {
		put(m, newID(kube.KindDeployment, "shop", "payments-api"), t0,
			map[string]inventory.Value{
				kube.AttrRelease: inventory.Text("payments"),
			})
	}
	return m, release
}
