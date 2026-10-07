package kube

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

var wakeNow = time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)

type wakeModel struct {
	t *testing.T
	m *inventory.Model
}

func newWakeModel(t *testing.T) wakeModel {
	return wakeModel{t, inventory.NewModel(inventory.Options{})}
}

func (w wakeModel) apply(o inventory.Observation) {
	w.t.Helper()
	o.Source = ObservationSource
	_, err := w.m.Apply(o)
	require.NoError(w.t, err)
}

// scale records a workload scaled from before to after replicas at at.
func (w wakeModel) scale(name, before, after string, at time.Time) {
	id := inventory.EntityID{Kind: KindDeployment, Namespace: "shop",
		Name: name}
	w.apply(inventory.Observation{Kind: inventory.Observed, At: at,
		Entity: id})
	w.apply(inventory.Observation{Kind: inventory.Changed, At: at,
		Entity: id, Change: inventory.Change{Entity: id, At: at,
			Fields: []inventory.FieldChange{{Path: "spec.replicas",
				Before: before, After: after}}}})
}

// start scales n workloads up from zero, one per minute from at.
func (w wakeModel) start(n int, at time.Time) {
	for i := range n {
		w.scale("app"+strconv.Itoa(i), "0", "2",
			at.Add(time.Duration(i)*time.Minute))
	}
}

func (w wakeModel) pod(
	name string, created time.Time, ready bool, warning bool,
) inventory.EntityID {
	id := inventory.EntityID{Kind: KindPod, Namespace: "shop", Name: name}
	w.apply(inventory.Observation{Kind: inventory.Observed, At: created,
		Entity: id, Attributes: map[string]inventory.Value{
			AttrCreated: inventory.Time(created),
			AttrReady:   inventory.Bool(ready)}})
	if warning {
		w.apply(inventory.Observation{Kind: inventory.Noted, At: created,
			Entity: id, Note: inventory.Note{At: created.Add(time.Minute),
				Source: "kubelet", Reason: "Unhealthy", Warning: true,
				Count: 3}})
	}
	return id
}

func TestLatestWakeNeedsManyWorkloadsStartingTogether(t *testing.T) {
	few := newWakeModel(t)
	few.start(WakeMinWorkloads-1, wakeNow)
	_, ok := LatestWake(few.m, wakeNow.Add(5*time.Minute))
	assert.False(t, ok, "four starts are a deploy, not a wake-up")

	many := newWakeModel(t)
	many.start(WakeMinWorkloads+1, wakeNow)
	w, ok := LatestWake(many.m, wakeNow.Add(5*time.Minute))
	require.True(t, ok)
	assert.Equal(t, wakeNow, w.Start)
	assert.Equal(t, wakeNow.Add(5*time.Minute), w.Last)
	assert.Len(t, w.Workloads, WakeMinWorkloads+1)
}

func TestScaleUpOfRunningWorkloadsIsNotAStart(t *testing.T) {
	m := newWakeModel(t)
	for i := range 8 {
		m.scale("app"+strconv.Itoa(i), "2", "4", wakeNow)
	}
	_, ok := LatestWake(m.m, wakeNow.Add(time.Minute))
	assert.False(t, ok)
}

func TestWakeEndsAfterAQuietSpellOrTheCap(t *testing.T) {
	m := newWakeModel(t)
	m.start(6, wakeNow)
	w, _ := LatestWake(m.m, wakeNow.Add(time.Minute))
	assert.Equal(t, wakeNow.Add(5*time.Minute+WakeQuiet), w.End())
	assert.Equal(t, 4*time.Minute, w.Remaining(
		wakeNow.Add(5*time.Minute+WakeQuiet-4*time.Minute)))
	assert.Zero(t, w.Remaining(w.End().Add(time.Second)))

	long := Wake{Start: wakeNow, Last: wakeNow.Add(time.Hour)}
	assert.Equal(t, wakeNow.Add(WakeMax), long.End())
}

func TestStartsFarApartAreSeparateWakes(t *testing.T) {
	m := newWakeModel(t)
	m.start(6, wakeNow)
	m.start(6, wakeNow.Add(2*time.Hour))
	w, ok := LatestWake(m.m, wakeNow.Add(2*time.Hour+10*time.Minute))
	require.True(t, ok)
	assert.Equal(t, wakeNow.Add(2*time.Hour), w.Start)
}

func TestNodesJoiningTogetherAreAWakeUp(t *testing.T) {
	m := newWakeModel(t)
	for i := range WakeMinNodes {
		m.apply(inventory.Observation{Kind: inventory.Observed,
			At: wakeNow, Entity: inventory.CoreID(KindNode, "",
				"n"+strconv.Itoa(i)),
			Attributes: map[string]inventory.Value{
				AttrCreated: inventory.Time(wakeNow)}})
	}
	w, ok := LatestWake(m.m, wakeNow.Add(time.Minute))
	require.True(t, ok)
	assert.Equal(t, WakeMinNodes, w.Nodes)
}

func TestPodWakeRemainingOnlyForPodsTheWakeCreated(t *testing.T) {
	m := newWakeModel(t)
	m.start(6, wakeNow)
	now := wakeNow.Add(7 * time.Minute)
	young := m.pod("young", wakeNow.Add(2*time.Minute), false, false)
	old := m.pod("old", wakeNow.Add(-time.Hour), false, false)
	pod := func(id inventory.EntityID) inventory.Entity {
		e, _ := m.m.Entity(id)
		return e
	}
	assert.Equal(t, 8*time.Minute, PodWakeRemaining(m.m, pod(young), now))
	assert.Zero(t, PodWakeRemaining(m.m, pod(old), now))
	assert.Zero(t, PodWakeRemaining(m.m, pod(young),
		wakeNow.Add(time.Hour)), "the wake-up is over")
}

func TestReportWakeCountsBlipsAndWhatIsStillFailing(t *testing.T) {
	m := newWakeModel(t)
	m.start(6, wakeNow)
	at := wakeNow.Add(2 * time.Minute)
	m.pod("a", at, true, true)
	m.pod("b", at, false, true)
	m.pod("c", at, true, false)
	w, _ := LatestWake(m.m, wakeNow.Add(time.Minute))
	report := ReportWake(m.m, w)
	assert.Equal(t, 2, report.Blips)
	assert.Equal(t, 1, report.Failing)
}

func TestContextWakeLastsAWhileAfterTheEnd(t *testing.T) {
	m := newWakeModel(t)
	m.start(6, wakeNow)
	end := wakeNow.Add(5*time.Minute + WakeQuiet)
	_, ok := ContextWake(m.m, end.Add(WakeContextTail-time.Minute))
	assert.True(t, ok)
	_, ok = ContextWake(m.m, end.Add(2*time.Hour))
	assert.False(t, ok)
}
