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

// notReadyNode is a NotReady node with the evidence its detector writes.
func notReadyNode(status, message string, more ...detection.Evidence,
) incident.Incident {
	id := inventory.CoreID(kube.KindNode, "", "ip-10-0-1-2")
	evidence := append([]detection.Evidence{
		{Label: "message", Value: message},
		{Label: detection.EvidenceReadyStatus, Value: status}}, more...)
	return incident.Incident{
		ID: "n-1", Root: id, Tier: incident.Page, State: incident.Open,
		Opened: writerNow.Add(-5 * time.Minute),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.NodeNotReady, Severity: detection.Critical,
			Since:    writerNow.Add(-5 * time.Minute),
			Summary:  "Node is NotReady for 5m0s",
			Evidence: evidence}),
	}
}

func TestWriteNodeNotReadyQuotesTheKubeletsWords(t *testing.T) {
	got := Writer{}.Write(announce(notReadyNode("False",
		"container runtime network not ready: NetworkReady=false "+
			"reason:NetworkPluginNotReady message:Network plugin returns "+
			"error: cni plugin not initialized")), writerNow).Note

	assert.Contains(t, got, "cni plugin not initialized")
	assert.NotContains(t, got, "cannot see why")
}

func TestWriteNodeNotReadyAlsoQuotesConditionsAndEvents(t *testing.T) {
	got := Writer{}.Write(announce(notReadyNode("False",
		"container runtime is down",
		detection.Evidence{Label: detection.EvidenceNodeCondition,
			Value: `DiskPressure "kubelet has disk pressure"`},
		detection.Evidence{Label: detection.EvidenceNodeEvent,
			Value: `ImageGCFailed "failed to get image stats"`},
		detection.Evidence{Label: detection.EvidenceNodeEvent,
			Value: `Rebooted "Node has been rebooted"`})),
		writerNow).Note

	assert.Contains(t, got, `It reports "container runtime is down" and `+
		`also DiskPressure "kubelet has disk pressure"; its recent `+
		`events include ImageGCFailed "failed to get image stats" and `+
		`Rebooted "Node has been rebooted".`)
}

func TestWriteNodeUnknownSaysKwatchCannotSeeWhy(t *testing.T) {
	got := Writer{}.Write(announce(notReadyNode("Unknown",
		"Kubelet stopped posting node status.")), writerNow).Note

	assert.Contains(t, got, "Kubelet stopped posting node status")
	assert.Contains(t, got, "kwatch cannot see why from inside the "+
		"cluster: the kubelet stopped reporting, so the node may be "+
		"down or unreachable.")
}
