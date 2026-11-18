package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// journalRig applies observations to a model through a LinkJournal, as
// the engine does.
type journalRig struct {
	t       *testing.T
	model   *inventory.Model
	journal *LinkJournal
	now     time.Time
}

func newJournalRig(t *testing.T) *journalRig {
	return &journalRig{t: t, model: inventory.NewModel(inventory.Options{}),
		journal: NewLinkJournal(),
		now:     time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)}
}

func (r *journalRig) apply(o inventory.Observation) {
	r.t.Helper()
	if o.Source == "" {
		o.Source = ObservationSource
	}
	o.At = r.now
	mark := r.journal.Before(r.model, o)
	_, err := r.model.Apply(o)
	require.NoError(r.t, err)
	r.journal.After(r.model, o, mark)
}

func (r *journalRig) observe(
	id inventory.EntityID, attrs map[string]inventory.Value,
) {
	r.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs})
}

func (r *journalRig) relate(
	id inventory.EntityID, rel inventory.RelationType,
	targets ...inventory.EntityID,
) {
	r.apply(inventory.Observation{Kind: inventory.Related, Entity: id,
		Relation: rel, Targets: targets})
}

// due moves the clock on by d and returns the recorded ends.
func (r *journalRig) due(d time.Duration) []inventory.Observation {
	r.now = r.now.Add(d)
	return r.journal.Due(r.model, r.now)
}

var (
	journalSvc = inventory.CoreID(KindService, "shop", "payments")
	journalAPI = inventory.CoreID(KindDeployment, "shop", "api")
	journalCfg = inventory.CoreID(KindConfigMap, "shop", "app-config")
)

func selectorAttrs(selector string) map[string]inventory.Value {
	return map[string]inventory.Value{
		AttrSelector: inventory.Text(selector)}
}

// routedRig has Service payments selecting deployment api.
func routedRig(t *testing.T) *journalRig {
	r := newJournalRig(t)
	r.observe(journalAPI, map[string]inventory.Value{
		AttrTemplateLabels: inventory.Text("app=api")})
	r.observe(journalSvc, selectorAttrs("app=api"))
	return r
}

func TestLinkJournalRecordsServiceThatStopsRouting(t *testing.T) {
	r := routedRig(t)
	r.observe(journalSvc, selectorAttrs("app=apii"))
	assert.Empty(t, r.due(30*time.Second), "waits out the flap window")
	got := r.due(30 * time.Second)
	require.Len(t, got, 1)
	assert.Equal(t, journalSvc, got[0].Entity)
	assert.Equal(t, inventory.Changed, got[0].Kind)
	assert.Equal(t, r.now.Add(-LinkFlapWindow), got[0].Change.At)
	rel, target, ok := got[0].Change.RemovedLink()
	require.True(t, ok)
	assert.Equal(t, inventory.RoutesTo, rel)
	assert.Equal(t, journalAPI, target)
	assert.Empty(t, r.due(time.Hour), "recorded once")
}

func TestLinkJournalIgnoresALinkBackWithinAMinute(t *testing.T) {
	r := routedRig(t)
	r.observe(journalSvc, selectorAttrs("app=apii"))
	r.now = r.now.Add(20 * time.Second)
	r.observe(journalSvc, selectorAttrs("app=api"))
	assert.Empty(t, r.due(time.Hour))
}

func TestLinkJournalRecordsWorkloadThatStopsUsingConfig(t *testing.T) {
	r := newJournalRig(t)
	r.observe(journalAPI, nil)
	r.relate(journalAPI, inventory.References, journalCfg)
	r.apply(inventory.Observation{Kind: inventory.Changed,
		Entity: journalAPI, Change: inventory.Change{At: r.now,
			Actor: "alice", Fields: []inventory.FieldChange{
				{Path: "spec.template.spec.volumes"}}}})
	r.relate(journalAPI, inventory.References)
	got := r.due(LinkFlapWindow)
	require.Len(t, got, 1)
	assert.Equal(t, journalAPI, got[0].Entity)
	assert.Equal(t, "alice", got[0].Change.Actor)
	rel, target, _ := got[0].Change.RemovedLink()
	assert.Equal(t, inventory.References, rel)
	assert.Equal(t, journalCfg, target)
}

// A pod's links end on every restart: they are never recorded.
func TestLinkJournalIgnoresPodChurn(t *testing.T) {
	r := newJournalRig(t)
	pod := inventory.CoreID(KindPod, "shop", "api-1")
	node := inventory.CoreID(KindNode, "", "n1")
	r.observe(pod, nil)
	r.relate(pod, inventory.References, journalCfg)
	r.relate(pod, inventory.RunsOn, node)
	r.relate(pod, inventory.Calls,
		inventory.CoreID(KindExternalEndpoint, "", "db:5432"))
	r.relate(pod, inventory.Calls)
	r.relate(pod, inventory.References)
	r.apply(inventory.Observation{Kind: inventory.Gone, Entity: pod})
	next := inventory.CoreID(KindPod, "shop", "api-2")
	r.observe(next, nil)
	r.relate(next, inventory.References, journalCfg)
	assert.Empty(t, r.due(time.Hour))
}

// A deleted holder's links end with it; the deletion is the change.
func TestLinkJournalIgnoresDeletedHolder(t *testing.T) {
	r := newJournalRig(t)
	r.observe(journalAPI, nil)
	r.relate(journalAPI, inventory.References, journalCfg)
	r.relate(journalAPI, inventory.References)
	r.apply(inventory.Observation{Kind: inventory.Gone, Entity: journalAPI})
	assert.Empty(t, r.due(time.Hour))
}

func TestLinkJournalRecordsPolicyThatStopsCovering(t *testing.T) {
	r := newJournalRig(t)
	policy := inventory.CoreID(KindNetworkPolicy, "shop", "allow-api")
	r.observe(journalAPI, map[string]inventory.Value{
		AttrTemplateLabels: inventory.Text("app=api")})
	r.observe(policy, selectorAttrs("<none>"))
	r.observe(policy, selectorAttrs("app=web"))
	got := r.due(LinkFlapWindow)
	require.Len(t, got, 1)
	rel, target, _ := got[0].Change.RemovedLink()
	assert.Equal(t, inventory.Selects, rel)
	assert.Equal(t, journalAPI, target)
}
