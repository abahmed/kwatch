package detectors

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A nearly full volume is advice: nothing failed yet. It becomes a
// failure once it is full or a pod that mounts it fails to write for
// lack of space.
func TestVolumeUsageIsAdviceUntilWritesFail(t *testing.T) {
	for name, tc := range map[string]struct {
		pct      float64
		logLine  string
		advisory bool
	}{
		"86% used": {86, "", true},
		"96% used": {96, "", true},
		"full":     {99.5, "", false},
		"pod says no space left": {90,
			"write /data/x: no space left on device", false},
		"pod fails for another reason": {90, "panic: boom", true},
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			pvc := newID(kube.KindPVC, "ns", "data")
			put(m, pvc, t0, map[string]inventory.Value{
				kube.AttrVolumeUsedPct: inventory.Number(tc.pct)})
			if tc.logLine != "" {
				pod := newID(kube.KindPod, "ns", "p")
				c := newID(kube.KindContainer, "ns", "p/app")
				put(m, pod, t0, nil)
				put(m, c, t0, map[string]inventory.Value{
					kube.AttrLastErrorLine: inventory.Text(tc.logLine)})
				link(m, pod, inventory.Mounts, pvc)
				relateEntity(m, c, inventory.PartOf, pod)
			}
			got := evaluate(VolumeUsage{}, m, t0, pvc, nil).Findings
			if len(got) != 1 || got[0].Advisory != tc.advisory {
				t.Fatalf("findings %+v, want one with Advisory=%v",
					got, tc.advisory)
			}
		})
	}
}
