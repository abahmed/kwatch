package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A claim a workload scaled to zero still names, or a StatefulSet's
// claim, is idle by design: never "unused". A truly unused claim is
// advice only, never an alert of its own.
func TestClaimUnusedIdleWorkloadsAndAdvice(t *testing.T) {
	for name, tc := range map[string]struct {
		claim  string
		setup  func(m *inventory.Model, claim inventory.EntityID)
		unused bool
	}{
		"deployment scaled to zero": {"data", func(
			m *inventory.Model, claim inventory.EntityID) {
			d := newID(kube.KindDeployment, "shop", "web")
			put(m, d, t0, map[string]inventory.Value{
				kube.AttrReplicas: inventory.Number(0)})
			link(m, d, inventory.References, claim)
		}, false},
		"statefulset claim": {"data-db-0", func(
			m *inventory.Model, _ inventory.EntityID) {
			put(m, newID(kube.KindStatefulSet, "shop", "db"), t0,
				map[string]inventory.Value{
					kube.AttrReplicas: inventory.Number(0)})
		}, false},
		"no one names it": {"data", func(*inventory.Model,
			inventory.EntityID) {
		}, true},
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			claim := newID(kube.KindPVC, "shop", tc.claim)
			put(m, claim, t0.Add(-30*24*time.Hour),
				map[string]inventory.Value{
					kube.AttrPhase: inventory.Text("Bound")})
			tc.setup(m, claim)
			r := detection.NewRegistry(nil, Claim{})
			unusedFindings(r, m, claim, t0)
			got := unusedFindings(r, m, claim, t0.Add(DefaultUnusedAfter))
			if (len(got) == 1) != tc.unused {
				t.Fatalf("findings %+v, unused want %v", got, tc.unused)
			}
			if tc.unused && !got[0].Advisory {
				t.Fatalf("an unused claim must be advice: %+v", got[0])
			}
		})
	}
}
