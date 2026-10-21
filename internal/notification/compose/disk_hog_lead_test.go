package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// diskHogIncident is a workload blamed for filling the disks of two
// nodes; the findings it groups are the nodes' own.
func diskHogIncident() incident.Incident {
	ingest := inventory.CoreID(kube.KindDeployment, "media", "ingest")
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	pressure := func(node inventory.EntityID) detection.Finding {
		return detection.Finding{Entity: node,
			Reason:   reasons.EvictionThresholdMet,
			Severity: detection.Critical, Since: at(0, 0),
			Summary: "Node crossed an eviction threshold"}
	}
	return incident.Incident{
		ID: "inc-hog", Root: ingest, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 2,
		Members: members(pressure(n1), pressure(n2)),
		Cause: &rootcause.CauseRecord{
			Rule: "workload-fills-disks", Mode: explain.ModeDiskHog,
			Root: ingest, Chain: []inventory.EntityID{ingest, n1},
			Score: 0.9, Proof: []rootcause.Proof{{
				Code: rootcause.ProofFillsDisks, Count: 2,
				Supports: true, Weight: 0.15}}},
	}
}

// The nodes' "crossed an eviction threshold" is the nodes' state, not
// the workload's, and no workloads depend on a workload that fills
// disks: the lead says what the workload does and what follows.
func TestDiskHogLeadSaysTheWorkloadFillsTheDisks(t *testing.T) {
	text := notification.Text(Writer{}.Write(
		announce(diskHogIncident()), at(6, 0)))

	if strings.Contains(text, "eviction threshold") ||
		strings.Contains(text, "depend on it") {
		t.Fatalf("a node's state or a dependents proof was said of the "+
			"workload:\n%s", text)
	}
	for _, want := range []string{"ingest", "fills the disks",
		"pods on those nodes are evicted",
		"Its pods hold most of the disk in use on two nodes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("lead lacks %q:\n%s", want, text)
		}
	}
}
