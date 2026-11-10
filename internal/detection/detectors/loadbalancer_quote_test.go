package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Whichever controller warns about a LoadBalancer Service that has no
// address, the Service's waiting finding quotes it: the warning is not
// also an unusual event. Once the Service has its address, a repeated
// warning is unusual again.
func TestLoadBalancerWarningOfAnyControllerIsQuoted(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindService, "shop", "web")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})
	noteEntity(m, id, "AllocationFailed", "no available IPs", 5, t0)
	at := t0.Add(time.Minute)

	assert.Empty(t, Event{}.Detect(
		testDetectorContext(m, at), entityOf(m, id)))

	put(m, id, at, map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(true),
	})
	require.Len(t, Event{}.Detect(
		testDetectorContext(m, at), entityOf(m, id)), 1)
}
