package release

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A release whose new revision serves every replica is not failed for
// an incident that already ended: a forward release that fixed an
// earlier break is what serves now. While the incident is open, or
// while the release does not serve, the verdict stands.
func TestServingReleaseIsNotFailedByAnEndedIncident(t *testing.T) {
	for name, tc := range map[string]struct {
		ready int
		state incident.State
		want  Verdict
	}{
		"serving, incident resolved": {1, incident.Resolved, Watching},
		"serving, incident open":     {1, incident.Open, Failed},
		"not ready, resolved":        {0, incident.Resolved, Down},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(kube.KindDeployment, "storage")
			w.status(1, tc.ready, 1, "")
			w.change(t0, "argocd", image("storage:1058", "storage:1059"))
			inc := incident.Incident{ID: "inc-1", Root: w.id,
				Tier: incident.Page, State: incident.Open,
				Opened: t0.Add(time.Minute),
				Members: map[detection.Key]detection.Finding{
					{Entity: w.id, Reason: reasons.CrashLoopBackOff}: {}}}
			w.incidents = []incident.Incident{inc}
			var b Book
			w.look(&b, t0.Add(2*time.Minute))
			inc.State, inc.Resolved = tc.state, t0.Add(3*time.Minute)
			if tc.state != incident.Resolved {
				inc.Resolved = time.Time{}
			}
			w.incidents = []incident.Incident{inc}
			w.look(&b, t0.Add(4*time.Minute))
			assert.Equal(t, tc.want, b.Records[0].Verdict)
		})
	}
}
