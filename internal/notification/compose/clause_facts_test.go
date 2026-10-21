package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A second clause that states the fact stays: "no pod ... has those
// labels" is what is wrong with a Service that selects a typo.
func TestPredicateKeepsASecondClauseThatStatesTheFact(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "shop", "api")

	got := predicate(svc, "Service api selects app=apii; no pod or "+
		"workload template in shop has those labels")

	assert.Equal(t, "selects app=apii, but no pod or workload template "+
		"in shop has those labels", got)
}

func TestPredicateStillDropsCommentaryAfterASemicolon(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n1")

	got := predicate(node, "Node is low on memory; pods may be evicted")

	assert.Equal(t, "is low on memory", got)
}

// A rate that is a whole number is said as one.
func TestPredicateSaysAWholeRateWithoutADecimal(t *testing.T) {
	api := kube.APIServer

	got := predicate(api, "API server is rejecting 12.0 requests a second "+
		"(priority level workload-low)")

	assert.Equal(t, "is rejecting 12 requests a second (priority level "+
		"workload-low)", got)
	assert.Equal(t, "is rejecting 12.5 requests a second",
		predicate(api, "API server is rejecting 12.5 requests a second"))
}
