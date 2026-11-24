package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/replay"
)

// incidentsRooted counts the incidents that reached anyone (not silent)
// by root, the ones under root first.
func incidentsRooted(result replay.Result, root string) (at, all int) {
	for _, p := range result.Incidents {
		if p.Tier == incident.Silent {
			continue
		}
		all++
		if p.Root.String() == root {
			at++
		}
	}
	return at, all
}

// A removed shared dependency is one incident for its 27 users, opened
// and resolved once, beside the incident of the workload that fails for
// its own reason; at most one page.
func TestRemovedDependencyIsOneIncident(t *testing.T) {
	result := replayNamed(t, "shared-dependency-removed-overnight")
	cache, _ := incidentsRooted(result, "service/data/cache")
	own, _ := incidentsRooted(result, "deployment/shop/reports")
	assert.Equal(t, 1, cache, pageTexts(result))
	assert.Equal(t, 1, own, pageTexts(result))
	assert.LessOrEqual(t, pagesRaised(result), 1, pageTexts(result))
	assert.Len(t, decisionsOf(result, incident.Announce, ""), 2,
		pageTexts(result))
}

// The pods of one node that stay without a network after the morning
// start-up are one incident, the node's, not one per workload.
func TestStuckYoungNodeIsOneIncident(t *testing.T) {
	result := replayNamed(t, "morning-sandbox-one-node-stuck")
	node, all := incidentsRooted(result, "node//m5")
	assert.Equal(t, 1, node, pageTexts(result))
	assert.Equal(t, 1, all, pageTexts(result))
	assert.Zero(t, pagesRaised(result), pageTexts(result))
}
