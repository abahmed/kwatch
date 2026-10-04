package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var workloadConfigRowCases = []rowCase{
	{row: "memory-limit-too-low", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			return oomCase(f, 40*time.Second)
		}},
	{row: "probe-port-mismatch", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 2)
			for _, pod := range pods {
				f.observe(containerOf(pod), map[string]inventory.Value{
					kube.AttrContainerPorts: inventory.Text("8080"),
					kube.AttrProbePorts:     inventory.Text("8081"),
				})
				f.fail(containerOf(pod), "Probe.Readiness", degradedH, 2,
					"dial tcp 10.0.0.5:8081: connect: connection refused")
			}
			return containerOf(pods[0])
		}},
	{row: "startup-budget-too-short", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 3)
			for _, pod := range pods[:2] {
				f.observe(pod, map[string]inventory.Value{
					kube.AttrReady:      inventory.Bool(true),
					kube.AttrStartTime:  inventory.Time(t0.Add(-time.Hour)),
					kube.AttrReadySince: inventory.Time(t0.Add(-58 * time.Minute)),
				})
			}
			f.observe(containerOf(pods[2]), map[string]inventory.Value{
				kube.AttrProbeBudget: inventory.Number(30),
			})
			f.fail(containerOf(pods[2]), "Probe.Startup", failingH, 2, "")
			return containerOf(pods[2])
		}},
	{row: "helper-container-blocks-pod",
		want: "container/shop/api-1-0/proxy",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "api", 1)[0]
			proxy := inventory.CoreID(kube.KindContainer, "shop",
				pod.Name+"/proxy")
			f.observe(proxy, map[string]inventory.Value{
				kube.AttrSidecar: inventory.Bool(true),
			})
			f.relate(proxy, inventory.PartOf, pod)
			f.fail(proxy, "CrashLoop", failingH, 1, "")
			f.fail(pod, "NotReady", degradedH, 2, "")
			return pod
		}},
	{row: "image-drift", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 2)
			f.fail(inventory.CoreID(kube.KindDeployment, "shop", "api"),
				"ImageDrift", degradedH, 1, "")
			f.fail(containerOf(pods[0]), "CrashLoop", failingH, 2, "")
			return containerOf(pods[0])
		}},
}

// observe sets attributes of id, creating it if needed.
func (f *fixture) observe(
	id inventory.EntityID, attrs map[string]inventory.Value,
) {
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs})
}

// oomCase OOM-kills both replicas of api after runs of the given length.
func oomCase(f *fixture, run time.Duration) inventory.EntityID {
	pods := f.workload("shop", "api", 2)
	for _, pod := range pods {
		f.observe(containerOf(pod), map[string]inventory.Value{
			kube.AttrLastStarted:  inventory.Time(t0.Add(time.Minute)),
			kube.AttrLastFinished: inventory.Time(t0.Add(time.Minute + run)),
		})
		f.fail(containerOf(pod), "OOMKilled", failingH, 2, "")
	}
	return containerOf(pods[0])
}

// TestMemoryLimitNeedsShortRuns: an OOM kill after hours of running is
// a leak, not a limit set too low.
func TestMemoryLimitNeedsShortRuns(t *testing.T) {
	f := newFixture(t)
	effect := oomCase(f, 3*time.Hour)
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Row == "memory-limit-too-low" {
		t.Fatalf("a long run blamed the limit: %+v", c)
	}
}

// TestHelperRowsAreInside: rows that blame the failure's own workload
// say so, and outside rows do not.
func TestHelperRowsAreInside(t *testing.T) {
	for row, want := range map[string]bool{
		"helper-container-blocks-pod": true, "memory-limit-too-low": true,
		"custom-resource-failing": true, "node-not-ready": false,
		"self": false,
	} {
		if InsideRow(row) != want {
			t.Errorf("InsideRow(%s) = %v, want %v", row, !want, want)
		}
	}
}
