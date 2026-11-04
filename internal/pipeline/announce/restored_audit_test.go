package announce

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func restoredDecision(
	i int, tier incident.Tier, found ...string,
) incident.Decision {
	root := inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: "shop", Name: fmt.Sprintf("app%d", i)}
	members := map[detection.Key]detection.Finding{}
	for _, reason := range found {
		members[detection.Key{Entity: root, Reason: reason}] =
			detection.Finding{Reason: reason}
	}
	return incident.Decision{Action: incident.Announce, Reason: "restored",
		Incident: incident.Incident{ID: fmt.Sprintf("inc-%d", i),
			Root: root, Tier: tier, Members: members}}
}

func TestRestoredListingNamesIncidentRootTierAndReasons(t *testing.T) {
	listed := restoredListing([]incident.Decision{
		restoredDecision(1, incident.Notify, "OOMKilled", "CrashLoop"),
		restoredDecision(2, incident.Digest, "NodePSIHigh"),
	})

	assert.Zero(t, listed.Opened, "restored incidents are not new")
	assert.Equal(t, 2, listed.Ongoing)
	assert.Equal(t, []string{
		"inc-1: deployment/shop/app1 tier=notify reasons=CrashLoop,OOMKilled",
		"inc-2: deployment/shop/app2 tier=digest reasons=NodePSIHigh",
	}, listed.Items)
}

// 552 restored incidents must not make a 552-item audit line: the first
// MaxRestoredAuditItems are named and the count says how many there are.
func TestRestoredListingIsBounded(t *testing.T) {
	var all []incident.Decision
	for i := range 120 {
		all = append(all, restoredDecision(i, incident.Digest, "X"))
	}

	listed := restoredListing(all)

	require.Len(t, listed.Items, MaxRestoredAuditItems)
	assert.Equal(t, 120, listed.Ongoing)
}

func TestRestoredTotalsCountByTier(t *testing.T) {
	got := restoredTotals([]incident.Decision{
		restoredDecision(1, incident.Page, "A"),
		restoredDecision(2, incident.Digest, "A"),
		restoredDecision(3, incident.Digest, "A"),
	})

	assert.Equal(t, []any{"incidents", 3, "page", 1, "notify", 0,
		"digest", 2, "silent", 0}, got)
}

// The lower-priority incidents the next digest carries are named too,
// after the listed ones, and the entry's tier is the loudest listed.
func TestRestoredListingNamesLowerPriorityAndLoudestTier(t *testing.T) {
	listed := []incident.Decision{
		restoredDecision(1, incident.Notify, "A"),
		restoredDecision(2, incident.Page, "B"),
	}
	lower := []incident.Decision{restoredDecision(3, incident.Digest, "C")}

	got := restoredListing(listed, lower...)

	assert.Equal(t, 2, got.Ongoing)
	require.Len(t, got.Items, 3)
	assert.Equal(t, "inc-3: deployment/shop/app3 tier=digest reasons=C"+
		" (lower priority, next digest)", got.Items[2])
	assert.Equal(t, incident.Page, loudest(listed))
}
