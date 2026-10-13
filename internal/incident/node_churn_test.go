package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// churnRig announces a notify incident for the pod web, resolves it, and
// then lets the node n1 leave the model at the returned time. A pod web
// is created on n2 at that time, as the controller does after a reclaim.
func churnRig(t *testing.T) (*rig, detection.Finding, time.Time) {
	t.Helper()
	r := newRig(t, Config{})
	web := podSig("web")
	node := entity(kube.KindNode, "n1")
	r.setAttrs(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	announced(t, r, web)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	wantAction(t, r.tick(at(2*time.Minute+DefaultHold)), Resolve,
		"healthy for 3m0s")

	gone := at(10 * time.Minute)
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Gone, Source: "test", At: gone, Entity: node})
	require.NoError(t, err)
	r.setAttrs(web.Entity, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(gone)})
	r.tick(gone) // the manager sees the node leave
	return r, web, gone
}

// A pod that a node's removal displaced fails for two minutes and is
// ready again: the resolved notify incident does not reopen.
func TestNodeChurnBlipDoesNotReopenAnIncident(t *testing.T) {
	r, web, gone := churnRig(t)

	r.raise(gone.Add(10*time.Second), web)
	wantNone(t, r.tick(gone.Add(10*time.Second+DefaultReviseSettle)))
	r.clear(gone.Add(2*time.Minute), web)
	wantNone(t, r.tick(gone.Add(2*time.Minute)))
	wantNone(t, r.tick(gone.Add(churnStart+time.Minute)))
	assert.Empty(t, r.m.churn.held, "the cleared finding is not held")
	assert.Equal(t, Resolved, r.of(web.Entity).State)
}

// The same pod still failing when the hold is over is reported, as an
// incident of its own and not as the old one failing again.
func TestNodeChurnLongFailureIsANewIncident(t *testing.T) {
	r, web, gone := churnRig(t)
	old := r.idOf(web.Entity)

	r.raise(gone.Add(10*time.Second), web)
	wantNone(t, r.tick(gone.Add(churnStart-time.Second)))
	r.tick(gone.Add(churnStart + time.Second))
	// The pod is past boot noise by now, so the incident is announced.
	ds := r.tick(gone.Add(20 * time.Minute))
	require.Len(t, ds, 1)
	assert.Equal(t, Announce, ds[0].Action)
	assert.NotEqual(t, old, ds[0].Incident.ID)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
}

// A failure on settled infrastructure, with no node leaving since the
// incident resolved, still continues it.
func TestSettledRecurrenceStillFailsAgain(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.setAttrs(entity(kube.KindNode, "n1"), map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	announced(t, r, web)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	r.tick(at(2*time.Minute + DefaultHold))

	again := at(20 * time.Minute)
	r.raise(again, web)
	wantAction(t, r.tick(again.Add(DefaultReviseSettle)), Update,
		ReasonFailingAgain)
}
