package scenarios

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
)

// replayWithDetail replays name with every sent message's steps stored
// on its incident, as the application stores them for the dashboard.
func replayWithDetail(t *testing.T, name string) replay.Result {
	t.Helper()
	log, e := loadScenario(t, name)
	opts := e.options(log.Start)
	deps := withSync(newDependencies(), opts)
	incidents := deps.Incidents
	deps.Sink = func(_ context.Context, d incident.Decision,
		m notification.Message) {
		incidents.StoreDetail(d.Incident.ID, incident.Detail{
			Steps: pipeline.FixSteps(m)})
	}
	result, err := replay.Run(context.Background(), log, deps, opts)
	require.NoError(t, err)
	return result
}

// When members leave a group for causes of their own, the group's
// title and steps are written again for the members it still has:
// they no longer name the ones that left.
func TestGroupTitleAndStepsFollowItsMembers(t *testing.T) {
	result := replayWithDetail(t, "readiness-outage-member-leaves")
	found := false
	for _, p := range result.Incidents {
		if p.Root.String() != "external-endpoint//10.0.9.40:5432" {
			continue
		}
		found = true
		require.NotEmpty(t, p.FullTitle())
		require.NotEmpty(t, p.Steps)
		for _, gone := range []string{"payments", "reports"} {
			if strings.Contains(p.FullTitle(), gone) {
				t.Errorf("title names %s: %s", gone, p.FullTitle())
			}
			for _, s := range p.Steps {
				if strings.Contains(s.Text+s.Command, gone) {
					t.Errorf("step names %s: %+v", gone, s)
				}
			}
		}
	}
	require.True(t, found, "the database's incident")
}
