package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// The leftovers of each removed controller are one open incident, at
// the digest tier, rooted at the controller; nothing about them is sent
// at Needs attention or above. A live failure beside them keeps its
// own incident.
func TestRemovedControllerLeftoversAreOneIncident(t *testing.T) {
	for name, want := range map[string]int{
		"removed-mesh-leftovers":           1,
		"removed-policy-engine-leftovers":  1,
		"removed-runner-leftovers":         1,
		"removed-operator-one-group":       1,
		"parked-routes-one-line":           1,
		"young-stuck-peer-joins-old-group": 1,
		"removed-mesh-beside-live-failure": 2,
	} {
		t.Run(name, func(t *testing.T) {
			_, e, result := replayScenario(t, labelledDir, name)
			live := map[string]incident.Tier{}
			for _, p := range result.Incidents {
				if p.State != incident.Resolved && len(p.Members) > 0 {
					live[p.Root.String()] = p.Tier
				}
			}
			assert.Len(t, live, want, "%v", live)
			root := e.Root
			if want > 1 {
				root = e.OtherRoots[0]
			}
			assert.Equal(t, incident.Digest, live[root], "%v", live)
			for _, d := range result.Decisions {
				if d.Incident.Root.String() == root {
					assert.Less(t, d.Incident.Tier, incident.Notify,
						"%s sent at %s", d.Reason, d.Incident.Tier)
				}
			}
		})
	}
}
