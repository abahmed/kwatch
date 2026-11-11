package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// nodeTwoFailures is a node under CPU pressure for longer, and out of
// pod IPs, with the cause naming the IP exhaustion.
func nodeTwoFailures() incident.Incident {
	node := inventory.CoreID("node", "", "n1")
	since := time.Date(2026, 1, 5, 6, 0, 0, 0, time.UTC)
	pressure := detection.Finding{Entity: node, Reason: reasons.NodePSIHigh,
		Severity: detection.Warning, Since: since,
		Mode:    detection.ModePressureStall,
		Summary: "Node is under CPU pressure: 40% of the time stalled"}
	ips := detection.Finding{Entity: node,
		Reason: reasons.NodePodIPExhausted, Severity: detection.Warning,
		Since: since.Add(time.Minute), Mode: detection.ModeNetworkIPExhausted,
		Summary: "Node has run out of pod IP addresses"}
	return incident.Incident{Root: node, State: incident.Open,
		Tier: incident.Notify, Opened: since,
		Members: map[detection.Key]detection.Finding{
			pressure.Key(): pressure, ips.Key(): ips},
		Cause: &rootcause.CauseRecord{Root: node, Rule: "self",
			Mode:    detection.ModeNetworkIPExhausted,
			Summary: "node n1 has run out of pod IP addresses"}}
}

// A node with two failures of its own is titled by the one its cause
// names, not by the older one.
func TestRootFindingFollowsTheCause(t *testing.T) {
	p := nodeTwoFailures()

	lead := rootFinding(p, sortedMembers(p))

	require.NotNil(t, lead)
	assert.Equal(t, reasons.NodePodIPExhausted, lead.Reason)
	title := NewWriter("", nil).ProblemTitle(p, nil, p.Opened)
	assert.NotContains(t, title, "CPU pressure")
}

// Without a cause the most severe, then the oldest, finding leads.
func TestRootFindingWithoutCauseKeepsTheOldest(t *testing.T) {
	p := nodeTwoFailures()
	p.Cause = nil

	lead := rootFinding(p, sortedMembers(p))

	require.NotNil(t, lead)
	assert.Equal(t, reasons.NodePSIHigh, lead.Reason)
}
