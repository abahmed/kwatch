package explain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A zone whose nodes are all young is booting: its nodes failing while
// they come up are not "the zone is failing". The same zone, grown old,
// is blamed again.
func TestBootingZoneIsNotBlamedForItsNodes(t *testing.T) {
	for name, age := range map[string]time.Duration{
		"booting": 2 * time.Minute, "settled": time.Hour} {
		f := newFixture(t)
		f.nodes("zone-a", "a1")
		nodes := f.nodes("zone-b", "b1", "b2")
		for _, node := range nodes {
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: node, Attributes: map[string]inventory.Value{
					kube.AttrCreated: inventory.Time(f.now.Add(-age))}})
			f.fail(node, "NotReady", failingH, 1, "")
		}

		got := causes(f.explain())

		assert.Equal(t, name == "settled", contains(got, "zone//zone-b"),
			name)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
