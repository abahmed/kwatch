package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A stated cause says when it began, so the incident layer can tell a
// revised cause from one that began long after people were told.
func TestExplainCauseReportsWhenItBegan(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 2, "api")
	f.fail(node, "NotReady", failingH, 7, "")
	for _, pod := range pods {
		f.fail(pod, "NotReady", failingH, 8, "")
	}

	c := requireCause(t, f.explain(), pods[0], node.String())

	if !c.Began.Equal(t0.Add(7 * time.Minute)) {
		t.Fatalf("began = %v, want the node failure at +7m", c.Began)
	}
}

// A zone or pool went wrong when its first node broke.
func TestGroupStartIsItsFirstBrokenNode(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	f.fail(nodes[0], "NotReady", failingH, 7, "")
	f.fail(nodes[1], "NotReady", failingH, 4, "")
	f.fail(nodes[2], "PressureStall", degradedH, 1, "")
	v := newView(f.snapshot())

	start := v.startOf(inventory.CoreID(kube.KindZone, "", "zone-a"))

	if !start.ok || !start.t.Equal(t0.Add(4*time.Minute)) {
		t.Fatalf("zone start = %+v, want the first failing node at +4m",
			start)
	}
}

// A long-standing degraded finding does not date the failure: a pod
// throttled for days still fails anew when its node breaks.
func TestExplainOldDegradedFindingDoesNotBlockNewCause(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 1, "api")
	f.fail(pods[0], "CPUThrottled", degradedH, -3000, "")
	f.fail(node, "NotReady", failingH, 60, "")
	f.fail(pods[0], "NotReady", failingH, 61, "")

	requireCause(t, f.explain(), pods[0], node.String())
}

// The trace lists, per failure, the upstream entities that were reached
// and showed nothing wrong, so a message can say what was checked.
func TestExplainTraceListsCheckedUpstreamEntities(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 1, "api")
	f.fail(pods[0], detection.ModeCrashLoop, failingH, 2, "")

	e := f.explain()

	if len(e.Areas) != 1 {
		t.Fatalf("areas = %d, want 1", len(e.Areas))
	}
	checked := e.Areas[0].Trace.Checked[pods[0]]
	found := false
	for _, id := range checked {
		found = found || id == node
	}
	if !found {
		t.Fatalf("the healthy node must be listed as checked: %v", checked)
	}
}
