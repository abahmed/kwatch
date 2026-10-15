package announce

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func baselineIncident(
	tier incident.Tier, severity detection.Severity, members int,
) incident.Incident {
	root := inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: "shop", Name: "app"}
	found := map[detection.Key]detection.Finding{}
	for i := range members {
		reason := fmt.Sprintf("R%d", i)
		found[detection.Key{Entity: root, Reason: reason}] =
			detection.Finding{Reason: reason, Severity: severity}
	}
	return incident.Incident{ID: "inc-1", Root: root, Tier: tier,
		Members: found}
}

type baselineRig struct {
	c      *Collector
	now    time.Time
	audits []incident.Decision
}

func newBaselineRig(t *testing.T) *baselineRig {
	t.Helper()
	r := &baselineRig{now: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)}
	r.c = New(Env{
		Write: func(
			incident.Decision, time.Time,
		) notification.Message {
			return notification.Message{}
		},
		Sink: func(
			_ context.Context, d incident.Decision, _ notification.Message,
		) {
			r.audits = append(r.audits, d)
		},
	})
	r.c.Startup.Baseline = true
	r.c.Startup.AwaitSync = true
	return r
}

func (r *baselineRig) collect(
	action incident.Action, p incident.Incident,
) []incident.Decision {
	rest, _ := r.c.CollectStartup(context.Background(), r.now,
		[]incident.Decision{{Action: action, Incident: p}})
	return rest
}

// baselined starts a rig whose incident p existed at start.
func baselined(t *testing.T, p incident.Incident) *baselineRig {
	r := newBaselineRig(t)
	require.Empty(t, r.collect(incident.Announce, p))
	require.Contains(t, r.c.Startup.baselined, p.ID)
	r.c.Startup.AwaitSync = false
	return r
}

func TestBaselinedUpdateWithoutEscalationStaysAbsorbed(t *testing.T) {
	base := baselineIncident(incident.Notify, detection.Warning, 2)
	r := baselined(t, base)

	assert.Empty(t, r.collect(incident.Update, base))
	same := baselineIncident(incident.Notify, detection.Warning, 1)
	assert.Empty(t, r.collect(incident.Update, same), "fewer members")
	assert.Contains(t, r.c.Startup.baselined, base.ID)
}

func TestBaselinedUpdateIsLetThroughWhenSeverityRises(t *testing.T) {
	r := baselined(t, baselineIncident(incident.Notify, detection.Warning, 1))

	worse := baselineIncident(incident.Notify, detection.Critical, 1)
	rest := r.collect(incident.Update, worse)

	require.Len(t, rest, 1)
	assert.Equal(t, incident.Announce, rest[0].Action,
		"nobody was told of the incident: the first message is full")
	assert.NotContains(t, r.c.Startup.baselined, worse.ID)
}

func TestBaselinedUpdateIsLetThroughWhenTierRises(t *testing.T) {
	r := baselined(t, baselineIncident(incident.Digest, detection.Warning, 1))

	rest := r.collect(incident.Update,
		baselineIncident(incident.Page, detection.Warning, 1))

	require.Len(t, rest, 1)
}

func TestBaselinedUpdateIsLetThroughWhenMembersGrow(t *testing.T) {
	r := baselined(t, baselineIncident(incident.Notify, detection.Warning, 1))

	rest := r.collect(incident.Update,
		baselineIncident(incident.Notify, detection.Warning, 3))

	require.Len(t, rest, 1)
	assert.Len(t, rest[0].Incident.Members, 3)
}

func TestEscalatedBaselineIncidentBehavesNormallyAfterwards(t *testing.T) {
	r := baselined(t, baselineIncident(incident.Notify, detection.Warning, 1))
	require.Len(t, r.collect(incident.Update,
		baselineIncident(incident.Notify, detection.Critical, 1)), 1)

	// Later updates, even a quieter one, are no longer absorbed.
	rest := r.collect(incident.Update,
		baselineIncident(incident.Notify, detection.Warning, 1))
	require.Len(t, rest, 1)
	assert.Equal(t, incident.Update, rest[0].Action)
}
