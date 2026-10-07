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

func TestWriteIPExhaustionOnOneNode(t *testing.T) {
	root := inventory.CoreID(kube.KindNode, "", "n1")
	got := Writer{}.Write(announce(incident.Incident{
		ID: "ip-2", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(ipNode("n1", "5")),
	}), writerNow).Note

	assert.Contains(t, got, "5 pods can't start on 1 node: no free pod IPs.")
}
