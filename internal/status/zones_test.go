package status

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

func podFindings(ids ...inventory.EntityID) []detection.Finding {
	var out []detection.Finding
	for _, id := range ids {
		out = append(out, detection.Finding{Entity: id,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Warning})
	}
	return out
}

func zoneByName(z Zones, name string) Zone {
	for _, item := range z.Items {
		if item.Name == name {
			return item
		}
	}
	return Zone{}
}

func TestZoneFailureIsConcentratedWhenOthersAreHealthy(t *testing.T) {
	m := testModel(t)
	var pods []inventory.EntityID
	for _, n := range []string{"b1", "b2", "b3"} {
		node := addNode(t, m, n, "zone-b", false)
		pods = append(pods, addPod(t, m, "p-"+n, node))
	}
	addNode(t, m, "a1", "zone-a", true)
	addNode(t, m, "c1", "zone-c", true)

	got := ZoneHealth(m, podFindings(pods...))

	if !got.Assessed {
		t.Fatal("two or more zones must be assessed")
	}
	b := zoneByName(got, "zone-b")
	if b.Nodes != 3 || b.NotReady != 3 || b.FailingPods != 3 ||
		!b.Concentrated {
		t.Fatalf("zone-b = %+v", b)
	}
	if a := zoneByName(got, "zone-a"); a.Concentrated || a.NotReady != 0 {
		t.Fatalf("zone-a = %+v", a)
	}
	want := "Zone zone-b: 3 of 3 nodes not ready; 3 failing pods are " +
		"all in this zone; the other zones are healthy."
	if text := zoneText(b); text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
}

func TestOneZoneIsNotAssessed(t *testing.T) {
	m := testModel(t)
	addNode(t, m, "a1", "zone-a", false)
	addNode(t, m, "a2", "zone-a", false)
	if got := ZoneHealth(m, nil); got.Assessed {
		t.Fatalf("a single zone cannot be told from the cluster: %+v", got)
	}
}

func TestFailuresInTwoZonesAreNotConcentrated(t *testing.T) {
	m := testModel(t)
	var pods []inventory.EntityID
	for i, zone := range []string{"zone-a", "zone-b", "zone-c"} {
		node := addNode(t, m, fmt.Sprintf("n%d", i), zone, true)
		if zone != "zone-c" {
			pods = append(pods, addPod(t, m, "p-"+zone, node),
				addPod(t, m, "q-"+zone, node))
		}
	}
	got := ZoneHealth(m, podFindings(pods...))
	for _, z := range got.Items {
		if z.Concentrated {
			t.Fatalf("%s claimed with failures in two zones", z.Name)
		}
	}
}

func TestAFewFailuresDoNotMakeAZoneTheStory(t *testing.T) {
	m := testModel(t)
	node := addNode(t, m, "a1", "zone-a", true)
	addNode(t, m, "b1", "zone-b", true)
	got := ZoneHealth(m, podFindings(addPod(t, m, "p", node)))
	if zoneByName(got, "zone-a").Concentrated {
		t.Fatal("one failing pod is not a zone problem")
	}
}

func TestAdvisoriesAndUnscheduledPodsAreNotZoneFailures(t *testing.T) {
	m := testModel(t)
	node := addNode(t, m, "a1", "zone-a", true)
	addNode(t, m, "b1", "zone-b", true)
	pod := addPod(t, m, "p", node)
	pending := inventory.EntityID{Kind: pod.Kind, Namespace: "shop",
		Name: "pending"}
	observe(t, m, pending, nil)
	findings := []detection.Finding{
		{Entity: pod, Reason: reasons.RiskSingleReplica, Advisory: true},
		{Entity: pending, Reason: reasons.CrashLoopBackOff},
	}
	got := ZoneHealth(m, findings)
	if a := zoneByName(got, "zone-a"); a.FailingPods != 0 {
		t.Fatalf("zone-a = %+v", a)
	}
}
