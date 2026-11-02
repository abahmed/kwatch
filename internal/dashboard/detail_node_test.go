package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// node adds node name with labels and extra attributes, in zone.
func (tm *testModel) node(
	name, labels, zone string, attrs map[string]inventory.Value,
) inventory.EntityID {
	id := inventory.CoreID(kube.KindNode, "", name)
	if attrs == nil {
		attrs = map[string]inventory.Value{}
	}
	if labels != "" {
		attrs[kube.AttrNodeLabels] = inventory.Text(labels)
	}
	tm.observe(id, attrs)
	if zone != "" {
		tm.relate(id, inventory.PartOf,
			inventory.CoreID(kube.KindZone, "", zone))
	}
	return id
}

func TestDetailNodesDescribeWhereThePodsRun(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	created := testEpoch.Add(-48 * time.Hour)
	cond := kube.AttrConditionPrefix
	n1 := tm.node("n1", "karpenter.sh/capacity-type=spot", "us-east-1a",
		map[string]inventory.Value{
			kube.AttrReady:        inventory.Bool(false),
			kube.AttrCreated:      inventory.Time(created),
			kube.AttrInstanceType: inventory.Text("m6i.large"),
			cond + "Ready":        inventory.Text("False"),
			cond + "Ready.reason": inventory.Text("KubeletNotReady"),
			cond + "Ready.message": inventory.Text(
				"PLEG is not healthy, token=abcdefghijklmnop1234"),
			cond + "MemoryPressure": inventory.Text("False"),
			cond + "DiskPressure":   inventory.Text("True"),
		})
	tm.relate(pod, inventory.RunsOn, n1)
	other := inventory.CoreID(kube.KindPod, "web", "other")
	tm.observe(other, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
		kube.AttrReady: inventory.Bool(false)})
	done := inventory.CoreID(kube.KindPod, "web", "done")
	tm.observe(done, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Succeeded"),
		kube.AttrReady: inventory.Bool(false)})
	tm.relate(other, inventory.RunsOn, n1)
	tm.relate(done, inventory.RunsOn, n1)

	d := detailOf(t, contextServer(tm.m, false,
		failingAt("n", root, testEpoch.Add(-time.Hour))), "n")
	if len(d.Nodes) != 1 {
		t.Fatalf("nodes = %+v", d.Nodes)
	}
	n := d.Nodes[0]
	if n.Name != "n1" || n.Zone != "us-east-1a" ||
		n.CapacityType != "spot" || n.InstanceType != "m6i.large" ||
		!n.Created.Equal(created) || n.Ready == nil || *n.Ready ||
		n.Pods != 1 || n.OtherFailing != 1 || n.Root {
		t.Errorf("node = %+v", n)
	}
	if len(n.Conditions) != 2 || n.Conditions[0].Type != "DiskPressure" ||
		n.Conditions[1].Type != "Ready" ||
		n.Conditions[1].Reason != "KubeletNotReady" ||
		strings.Contains(n.Conditions[1].Message, "abcdefghijklmnop1234") {
		t.Errorf("conditions = %+v", n.Conditions)
	}
}

func TestDetailNodesOfARootNode(t *testing.T) {
	tm := newTestModel(t)
	n := tm.node("n2", "eks.amazonaws.com/capacityType=ON_DEMAND", "",
		map[string]inventory.Value{
			kube.AttrUnschedulable:             inventory.Bool(true),
			kube.AttrConditionPrefix + "Ready": inventory.Text("True"),
		})
	p := failingAt("r", n, testEpoch.Add(-time.Hour))
	d := detailOf(t, contextServer(tm.m, false, p), "r")
	if len(d.Nodes) != 1 || !d.Nodes[0].Root ||
		d.Nodes[0].CapacityType != "on-demand" || !d.Nodes[0].Cordoned ||
		len(d.Nodes[0].Conditions) != 0 || d.Nodes[0].Pods != 0 {
		t.Fatalf("nodes = %+v", d.Nodes)
	}
}

func TestDetailNodesAreBounded(t *testing.T) {
	tm := newTestModel(t)
	root, _ := tm.deployment()
	for i := range maxNodes + 2 {
		pod := tm.pod(root, fmt.Sprintf("p%d", i), nil)
		tm.relate(pod, inventory.RunsOn,
			tm.node(fmt.Sprintf("n%d", i), "", "", nil))
	}
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("b", root, testEpoch.Add(-time.Hour))), "b")
	if len(d.Nodes) != maxNodes || d.Nodes[0].Name != "n0" ||
		d.Truncated == nil || d.Truncated.Nodes != 2 {
		t.Fatalf("nodes = %+v, truncated %+v", d.Nodes, d.Truncated)
	}
	none := incident.Incident{ID: "x", Root: inventory.CoreID(
		kube.KindConfigMap, "shop", "cfg"), Opened: testEpoch}
	if rows, cut := nodesOf(tm.m, none); rows != nil || cut != 0 {
		t.Fatalf("an incident without pods has nodes: %+v", rows)
	}
}

func TestCapacityTypeReadsTheWellKnownLabels(t *testing.T) {
	cases := map[string]string{
		"karpenter.sh/capacity-type=on-demand":       "on-demand",
		"eks.amazonaws.com/capacityType=SPOT":        "spot",
		"cloud.google.com/gke-spot=true":             "spot",
		"cloud.google.com/gke-preemptible=true":      "spot",
		"kubernetes.azure.com/scalesetpriority=spot": "spot",
		"cloud.google.com/gke-spot=false":            "",
		"team=payments":                              "",
	}
	for labels, want := range cases {
		if got := capacityType(kube.ParseLabels(labels)); got != want {
			t.Errorf("%s: %q, want %q", labels, got, want)
		}
	}
}
