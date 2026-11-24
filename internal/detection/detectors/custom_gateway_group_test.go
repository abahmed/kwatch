package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// acceptedUnknown is a custom resource of group whose controller has
// left Accepted=Unknown since t0, evaluated an hour later.
func acceptedUnknown(t *testing.T, group string) []string {
	t.Helper()
	m := newTestModel()
	id := inventory.NewEntityID(group, "gateway", "default", "edge")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom: inventory.Bool(true),
	})
	setCondition(m, id, "Accepted", "Unknown", "Pending", "", t0)
	var out []string
	for _, f := range evaluate(Custom{}, m, t0.Add(time.Hour), id,
		nil).Findings {
		out = append(out, f.Summary)
	}
	for _, f := range evaluate(Generic{}, m, t0.Add(time.Hour), id,
		nil).Findings {
		out = append(out, f.Summary)
	}
	return out
}

// Accepted=Unknown is a Gateway API object waiting for a controller; on
// another API group's kind of the same name it means nothing.
func TestAcceptedUnknownIsOnlyAGatewayAPIState(t *testing.T) {
	got := acceptedUnknown(t, kube.GatewayGroup)
	require.NotEmpty(t, got)
	assert.Contains(t, got[0], "Accepted=Unknown")

	assert.Empty(t, acceptedUnknown(t, "example.com"))
}

// Only a Gateway API object waits the short unclaimed window.
func TestUnclaimedGraceIsOnlyForGatewayAPI(t *testing.T) {
	failing := []failingCondition{{label: "Accepted=Unknown",
		unknown:   true,
		unclaimed: gatewayAcceptance("example.com", "Accepted")}}
	assert.Equal(t, DefaultGenerationGrace, failingGrace(failing))

	failing[0].unclaimed = gatewayAcceptance(kube.GatewayGroup, "Accepted")
	assert.Equal(t, DefaultGatewayUnclaimed, failingGrace(failing))
}
