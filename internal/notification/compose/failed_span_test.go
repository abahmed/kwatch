package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A node whose pressure lasted a few minutes is not "failing for 15
// minutes" because the eviction its pods suffered stayed in the event
// window: the span is the node's own, ended by the condition clearing.
func TestNodeResolveCountsTheNodesOwnPressureNotTheLingeringEviction(
	t *testing.T,
) {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	const threshold = "Node crossed an eviction threshold; the kubelet is " +
		"reclaiming resources"
	p := incident.Incident{ID: "inc-n", Root: node, Tier: incident.Notify,
		State: incident.Resolved, Opened: at(0, 0),
		RecoveringSince: at(15, 0), Resolved: at(18, 0),
		Cause: &rootcause.CauseRecord{Rule: "node-disk-pressure", Root: node,
			RootFindings: []detection.Finding{{Entity: node,
				Reason:  reasons.EvictionThresholdMet,
				Mode:    detection.ModeDiskEvictionThreshold,
				Summary: threshold}}},
		Timeline: []incident.Event{
			{At: at(0, 0), Text: threshold + " (node n1)",
				Entity: aboutEntity(node)},
			{At: at(1, 0), Text: "Node is low on disk space (node n1)",
				Entity: aboutEntity(node)},
			{At: at(2, 35), Text: incident.RecoveredPrefix +
				"Node is low on disk space (node n1)",
				Entity: aboutEntity(node)},
			{At: at(15, 0), Text: incident.RecoveredPrefix + threshold +
				" (node n1)", Entity: aboutEntity(node)},
		}}

	msg := Writer{}.Write(incident.Decision{Action: incident.Resolve,
		Incident: p, Reason: "healthy for 3m0s"}, at(18, 0))

	assert.Contains(t, msg.Note, "It was failing for about three minutes.")
	assert.NotContains(t, msg.Note, "15 minutes")
}

func TestResolveKeepsTheWholeSpanWithoutALingeringEvent(t *testing.T) {
	p := podCrash(incident.Page)
	p.State, p.Resolved = incident.Resolved, writerNow

	msg := Writer{}.Write(incident.Decision{Action: incident.Resolve,
		Incident: p}, writerNow)

	assert.Contains(t, msg.Note, "It was down for one hour.")
}
