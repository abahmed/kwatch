package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// journaled is an incident whose journal holds a break, two
// transitions, its timers and an acknowledgement.
func journaled() incident.Incident {
	p := testIncident("j", "shop", -10*time.Minute)
	p.State = incident.Recovering
	p.RecoveringSince = testEpoch.Add(-time.Minute)
	ready, code := false, 137
	row := incident.ContainerState{Namespace: "shop", Pod: "api-1",
		Container: "api", Ready: &ready, Restarts: 4,
		LastReason: "OOMKilled", LastExitCode: &code, MemoryLimit: 1 << 28}
	p.Journal.Breaks = []incident.BreakSnapshot{
		{At: p.Opened, Reason: incident.BreakFirst,
			Containers: []incident.ContainerState{row}},
		{At: testEpoch.Add(-5 * time.Minute), Reason: incident.BreakWorse,
			Containers: []incident.ContainerState{row}},
	}
	p.Journal.Transitions = []incident.Transition{
		{At: p.Opened, Kind: incident.TransitionOpened, To: "settling"},
		{At: testEpoch.Add(-5 * time.Minute),
			Kind: incident.TransitionTierRaised, From: "notify",
			To: "page"},
	}
	p.Journal.Timers = incident.Timers{Hold: 3 * time.Minute,
		Next: testEpoch.Add(2 * time.Minute)}
	p.Ack = &incident.Ack{On: p.Root, Note: "on it",
		Since: testEpoch.Add(-3 * time.Minute), By: "kubectl-annotate"}
	return p
}

func TestDetailServesTheJournal(t *testing.T) {
	d := detailOf(t, contextServer(nil, false, journaled()), "j")

	require.Len(t, d.Snapshots, 2)
	require.NotNil(t, d.Snapshot)
	assert.Equal(t, incident.BreakWorse, d.Snapshot.Reason)
	assert.Equal(t, 4, d.Snapshot.Containers[0].Restarts)
	assert.Equal(t, "OOMKilled", d.Snapshot.Containers[0].LastReason)

	require.Len(t, d.Transitions, 2)
	assert.Equal(t, Transition{At: testEpoch.Add(-5 * time.Minute),
		Kind: "tier raised", From: "notify", To: "page"}, d.Transitions[1])

	require.NotNil(t, d.Why)
	assert.Equal(t, 180, d.Why.HoldTotal)
	assert.Equal(t, 60, d.Why.HoldElapsed)
	assert.Equal(t, "Looks better; closes after 3m healthy, 1m so far",
		d.Why.StillOpenReason)
	assert.False(t, d.Why.CannotSee)

	require.NotNil(t, d.Ack)
	assert.Equal(t, AckDetail{Note: "on it",
		Since: testEpoch.Add(-3 * time.Minute), By: "kubectl-annotate",
		On: "Deployment/shop/app-j"}, *d.Ack)
	assert.True(t, d.Acked)
}

func TestDetailOfAnOlderIncidentHasNoJournal(t *testing.T) {
	d := detailOf(t, contextServer(nil, false,
		testIncident("o", "shop", -time.Minute)), "o")
	assert.Nil(t, d.Snapshot)
	assert.Empty(t, d.Transitions)
	assert.Nil(t, d.Ack)
	assert.False(t, d.Acked)
}

func TestDetailChangesPreferStoredAndMergeLive(t *testing.T) {
	tm, p := changesFixture(t)
	onset := p.Onset()
	gone := inventory.Change{
		Entity: inventory.CoreID(kube.KindSecret, "shop", "db"),
		At:     onset.Add(-3 * time.Minute), Actor: "helm",
		Fields: []inventory.FieldChange{
			{Path: "data.password", After: "changed"}}}
	p.Journal.Changes = []inventory.Change{incident.SafeChange(gone)}
	d := detailOf(t, contextServer(tm.m, true, p), "c")

	require.Len(t, d.Changes, 4)
	assert.Equal(t, "db", d.Changes[0].Name, "stored first")
	assert.Equal(t, []FieldDiff{{Path: "data.password", After: "changed"}},
		d.Changes[0].Fields)
	var cm ChangeItem
	for _, c := range d.Changes {
		if c.Name == "app-config" {
			cm = c
		}
	}
	assert.Equal(t, []FieldDiff{{Path: "data.DB_PASSWORD",
		After: "changed"}}, cm.Fields,
		"a data key never shows its values, only how it was edited")
	last := d.Changes[3]
	assert.Equal(t, "during", last.When)
	assert.Equal(t, []FieldDiff{{Path: "containers[api].image",
		Before: "reg.io/team/api:2", After: "reg.io/team/api:1"}},
		last.Fields)
}

func TestSimilarKeyPrefersTheStoredKey(t *testing.T) {
	p := testIncident("s", "shop", 0)
	p.Mode = detection.Mode("ImagePull")
	p.Cause = &rootcause.CauseRecord{Rule: "registry-down"}
	assert.Equal(t, "ImagePull\x00registry-down", similarKey(p))
	p.Journal.ErrorKey = incident.SameErrorKey("ImagePull", "")
	assert.Equal(t, "ImagePull\x00", similarKey(p))
}

func TestActivityCountsEveryTierRiseAsUrgent(t *testing.T) {
	p := testIncident("t", "shop", -30*time.Minute)
	p.Timeline = []incident.Event{{At: before(40 * time.Minute),
		Text: "still failing: paging again"}}
	p.Journal.Transitions = []incident.Transition{
		{At: before(30 * time.Minute), Kind: incident.TransitionOpened},
		{At: before(20 * time.Minute), Kind: incident.TransitionTierRaised},
		{At: before(10 * time.Minute), Kind: incident.TransitionTierLowered},
	}
	got := buildActivity([]incident.Incident{p}, testEpoch, time.Hour,
		redact.Evidence, true)
	assert.Equal(t, "t:urgent@20m0s t:new@30m0s t:urgent@40m0s",
		activityLine(got), "the older timeline still counts")
}
