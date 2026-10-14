package scenarios

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

func textsOf(result replay.Result) []string {
	var out []string
	for _, m := range result.Messages {
		out = append(out, notification.Text(m))
	}
	return out
}

// A lost node has no owner and no namespace of its own. Each team that
// has pods on it is told once when it is lost and once when it is back.
func TestNodeLostReachesEveryOwnerOnce(t *testing.T) {
	result := replayNamed(t, "node-lost-owner-routed")

	for _, team := range []string{"team-0", "team-1", "team-2", "team-3"} {
		announces, resolves := 0, 0
		for i, m := range result.Messages {
			if !slices.Contains(m.Route.Owners, team) {
				continue
			}
			switch result.Decisions[i].Action {
			case incident.Announce:
				announces++
			case incident.Resolve:
				resolves++
			}
		}
		assert.Equal(t, 1, announces, "%s announces", team)
		assert.Equal(t, 1, resolves, "%s resolves", team)
	}
	first := result.Messages[0].Route
	assert.Equal(t, slices.Sorted(slices.Values(groupNamespaces)),
		slices.Sorted(slices.Values(first.Namespaces)))
}

// Two lost nodes are two nodes, however many findings each has.
func TestZoneFailureCountsNodesNotFindings(t *testing.T) {
	result := replayNamed(t, "zone-failure")

	text := textsOf(result)[0]
	assert.Regexp(t, "(?i)two nodes", text)
	assert.NotContains(t, text, "four nodes")
}

// The agent's own crash line is the cause; the node message quotes it.
func TestNodeNetworkMessageQuotesTheAgentCrash(t *testing.T) {
	result := replayNamed(t, "cni-agent-crash-quoted")

	var node []string
	for i, m := range result.Messages {
		if result.Decisions[i].Incident.Root.Kind == "node" {
			node = append(node, notification.Text(m))
		}
	}
	assert.NotEmpty(t, node)
	assert.Contains(t, strings.Join(node, "\n"),
		"failed to initialize ipamd: no IP addresses available")
}

// Without the DNS probe, clients' name-resolution errors still make
// cluster DNS the one root; CoreDNS is a member, not an incident.
func TestCorednsFoldsUnderClusterDNSWithoutTheProbe(t *testing.T) {
	result := replayNamed(t, "coredns-down-no-probe")

	for i, d := range result.Decisions {
		if d.Action == incident.Announce {
			assert.Equal(t, "cluster-dns", string(d.Incident.Root.Kind),
				"announce %d: %s", i, textsOf(result)[i])
		}
	}
}

// A dial to a Service's ClusterIP is a dial to the Service.
func TestClusterIPDialIsNotAnExternalEndpoint(t *testing.T) {
	result := replayNamed(t, "dependency-clusterip-dial")

	for i, text := range textsOf(result) {
		assert.NotContains(t, text, "external endpoint", "message %d", i)
	}
}

// Updates name workloads, as announcements do, never pods or
// containers.
func TestUpdatesNameWorkloadsNotPods(t *testing.T) {
	result := replayNamed(t, "dependency-clusterip-dial")

	for i, text := range textsOf(result) {
		assert.NotContains(t, text, "container app in pod", "message %d", i)
		assert.NotContains(t, text, "pods billing-app", "message %d", i)
	}
}
