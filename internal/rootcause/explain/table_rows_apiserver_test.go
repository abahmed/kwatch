package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An API server that throttles, or answers reads slowly, explains the
// controllers that stopped reconciling and the autoscalers that cannot
// update their target.
func TestAPIServerLoadExplainsStalledControllers(t *testing.T) {
	cases := []struct{ api, effect detection.Mode }{
		{detection.ModeThrottlingAPIServer, detection.ModeNotReconciling},
		{detection.ModeLatencyReads, detection.ModeNotReconciling},
		{detection.ModeThrottlingAPIServer, detection.ModeScalingError},
	}
	for _, tc := range cases {
		apiMode, effectMode := tc.api, tc.effect
		t.Run(string(apiMode)+"/"+string(effectMode), func(t *testing.T) {
			f := newFixture(t)
			f.fail(kube.APIServer, apiMode, failingH, 1, "")
			var hpa inventory.EntityID
			for _, name := range []string{"api", "web"} {
				hpa = inventory.CoreID(kube.KindHPA, "shop", name)
				f.add(hpa)
				f.fail(hpa, effectMode, failingH, 2, "")
			}

			c := requireCause(t, f.explain(), hpa, "apiserver//kube-apiserver")
			if c.Row != "apiserver-unavailable" {
				t.Fatalf("row = %s", c.Row)
			}
		})
	}
}
