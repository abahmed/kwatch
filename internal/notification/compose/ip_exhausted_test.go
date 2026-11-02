package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const ipLine = "failed to assign an IP address to container"

func ipNode(name, pods string) detection.Finding {
	return detection.Finding{
		Entity: inventory.CoreID(kube.KindNode, "", name),
		Reason: reasons.NodePodIPExhausted, Severity: detection.Critical,
		Since:   writerNow.Add(-time.Hour),
		Summary: "Node has run out of pod IP addresses",
		Evidence: []detection.Evidence{
			{Label: "affected pods", Value: pods},
			{Label: detection.EvidenceSandboxEvent, Value: ipLine}},
	}
}

func TestWriteIPExhaustionCountsPodsAndNodesAndQuotes(t *testing.T) {
	root := inventory.CoreID(kube.KindNode, "", "n1")
	got := Writer{}.Write(announce(incident.Incident{
		ID: "ip-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(ipNode("n1", "5"), ipNode("n2", "5"),
			ipNode("n3", "4")),
	}), writerNow).Note

	assert.Contains(t, got, "14 pods can't start on 3 nodes: no free "+
		"pod IPs. Their events say \""+ipLine+"\".")
}

// The lead already names the one node: the count does not repeat it.
func TestWriteIPExhaustionOnTheRootNodeOmitsTheNodeCount(t *testing.T) {
	root := inventory.CoreID(kube.KindNode, "", "n1")
	got := Writer{}.Write(announce(incident.Incident{
		ID: "ip-2", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(ipNode("n1", "5")),
	}), writerNow).Note

	assert.Contains(t, got, "5 pods can't start: no free pod IPs.")
	assert.NotContains(t, got, "on 1 node")
}

func ipNodeStuck(name, pods, stuck string) detection.Finding {
	f := ipNode(name, pods)
	f.Evidence = append(f.Evidence, detection.Evidence{
		Label: detection.EvidenceStuckPods, Value: stuck})
	return f
}

func TestWriteIPExhaustionCountsOnlyPodsStillWaiting(t *testing.T) {
	root := inventory.CoreID(kube.KindNode, "", "n1")
	got := Writer{}.Write(announce(incident.Incident{
		ID: "ip-3", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(ipNodeStuck("n1", "42", "30")),
	}), writerNow).Note

	assert.Contains(t, got, "30 pods can't start: no free pod IPs; 12 "+
		"more failed to get one recently and have started since.")
}

func TestWriteIPExhaustionSaysWhenAllPodsStarted(t *testing.T) {
	root := inventory.CoreID(kube.KindZone, "", "zone-a")
	got := Writer{}.Write(announce(incident.Incident{
		ID: "ip-4", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(ipNodeStuck("n1", "4", "0"),
			ipNodeStuck("n2", "3", "0")),
	}), writerNow).Note

	assert.Contains(t, got, "7 pods on 2 nodes failed to get a pod IP "+
		"recently; all have started since.")
}
