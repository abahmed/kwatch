package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const probeTimeout = "Readiness probe failed: Get \"http://10.0.0.7:8080/" +
	"ready\": context deadline exceeded"

var scalingRowCases = []rowCase{
	{row: "autoscaling-limit",
		want: "horizontalpodautoscaler/shop/web",
		build: func(f *fixture) inventory.EntityID {
			pods := ceilingCase(f)
			for _, pod := range pods {
				f.fail(pod, "NotReady", degradedH, 3, probeTimeout)
			}
			return pods[0]
		}},
	{row: "autoscaling-limit-unavailable",
		want: "horizontalpodautoscaler/shop/web",
		build: func(f *fixture) inventory.EntityID {
			ceilingCase(f)
			deployment := inventory.CoreID(kube.KindDeployment, "shop",
				"web")
			f.fail(deployment, "Unavailable", degradedH, 3, "")
			return deployment
		}},
}

// ceilingCase is the web Deployment, scaled by an HPA at its maximum
// that wants more replicas.
func ceilingCase(f *fixture) []inventory.EntityID {
	pods := f.workload("shop", "web", 3)
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	f.add(hpa)
	f.relate(hpa, inventory.Scales,
		inventory.CoreID(kube.KindDeployment, "shop", "web"))
	f.fail(hpa, detection.ModeScalingMaxedOut, degradedH, 1, "")
	return pods
}

// TestAutoscalingLimitNeedsBusyPods: pods that refuse connections are
// not saturated, they are down, so the ceiling does not explain them.
func TestAutoscalingLimitNeedsBusyPods(t *testing.T) {
	f := newFixture(t)
	pods := ceilingCase(f)
	for _, pod := range pods {
		f.fail(pod, "NotReady", degradedH, 3, "Readiness probe failed: "+
			"dial tcp 10.0.0.7:8080: connect: connection refused")
	}
	if c, ok := f.explain().CauseOf(pods[0]); ok &&
		c.Row == "autoscaling-limit" {
		t.Fatalf("refused probes blamed on the ceiling: %+v", c)
	}
}

// TestHealthyAutoscalerIsNotFollowed: an autoscaler without a finding is
// never a cause, so no hop leads to it.
func TestHealthyAutoscalerIsNotFollowed(t *testing.T) {
	f := newFixture(t)
	f.workload("shop", "web", 1)
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	f.add(hpa)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.relate(hpa, inventory.Scales, deployment)
	v := newView(f.snapshot())
	if hops := v.scalerHops(deployment); len(hops) != 0 {
		t.Fatalf("hops = %+v, want none", hops)
	}
}
