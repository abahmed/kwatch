package announce

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

var wakeAt = time.Date(2026, 10, 30, 12, 22, 0, 0, time.UTC)

func applyWake(t *testing.T, m *inventory.Model, o inventory.Observation) {
	t.Helper()
	o.Source = kube.ObservationSource
	_, err := m.Apply(o)
	require.NoError(t, err)
}

// wakeModel has n workloads scaled up from zero and nodes nodes joining
// at wakeAt, and one pod of app0 with a startup warning.
func wakeModel(t *testing.T, n, nodes int) *inventory.Model {
	m := strayPodWakeModel(t, n, nodes)
	applyWake(t, m, inventory.Observation{Kind: inventory.Related,
		At: wakeAt, Entity: wakePod, Relation: inventory.OwnedBy,
		Targets: []inventory.EntityID{{Kind: kube.KindDeployment,
			Namespace: "shop", Name: "app0"}}})
	return m
}

var wakePod = inventory.EntityID{Kind: kube.KindPod, Namespace: "shop",
	Name: "p"}

// strayPodWakeModel is wakeModel with the warning pod owned by none of
// the workloads that started.
func strayPodWakeModel(t *testing.T, n, nodes int) *inventory.Model {
	m := inventory.NewModel(inventory.Options{})
	for i := range n {
		id := inventory.EntityID{Kind: kube.KindDeployment,
			Namespace: "shop", Name: "app" + strconv.Itoa(i)}
		applyWake(t, m, inventory.Observation{Kind: inventory.Observed,
			At: wakeAt, Entity: id})
		applyWake(t, m, inventory.Observation{Kind: inventory.Changed,
			At: wakeAt, Entity: id, Change: inventory.Change{Entity: id,
				At: wakeAt, Fields: []inventory.FieldChange{{
					Path: "spec.replicas", Before: "0", After: "1"}}}})
	}
	for i := range nodes {
		applyWake(t, m, inventory.Observation{Kind: inventory.Observed,
			At: wakeAt, Entity: inventory.CoreID(kube.KindNode, "",
				"n"+strconv.Itoa(i)),
			Attributes: map[string]inventory.Value{
				kube.AttrCreated: inventory.Time(wakeAt)}})
	}
	pod := wakePod
	applyWake(t, m, inventory.Observation{Kind: inventory.Observed,
		At: wakeAt, Entity: pod, Attributes: map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(wakeAt),
			kube.AttrReady:   inventory.Bool(false)}})
	applyWake(t, m, inventory.Observation{Kind: inventory.Noted,
		At: wakeAt, Entity: pod, Note: inventory.Note{
			At: wakeAt.Add(time.Minute), Source: "kubelet",
			Reason: "Unhealthy", Warning: true, Count: 3}})
	return m
}

func TestCollectorWakeNeedsStartedWorkloads(t *testing.T) {
	c := New(Env{History: wakeModel(t, 0, kube.WakeMinNodes)})
	c.noteWake(wakeAt.Add(-time.Hour))
	c.noteWake(wakeAt.Add(time.Hour))
	assert.Nil(t, c.wake, "nodes joined but no workload started: "+
		"no \"0 workloads started\" line")
}

func TestCollectorWakeBegunBeforeKwatchLookedIsNotSummarised(t *testing.T) {
	c := New(Env{History: wakeModel(t, kube.WakeMinWorkloads, 0)})
	c.noteWake(wakeAt.Add(3 * time.Minute))
	c.noteWake(wakeAt.Add(time.Hour))
	assert.Nil(t, c.wake)

	seen := New(Env{History: wakeModel(t, kube.WakeMinWorkloads, 0)})
	seen.noteWake(wakeAt.Add(-time.Minute))
	seen.noteWake(wakeAt.Add(time.Hour))
	require.NotNil(t, seen.wake)
	assert.Equal(t, kube.WakeMinWorkloads, seen.wake.Started)
}

// A pod that failed at startup but belongs to none of the workloads the
// wake-up started is not one of its brief startup failures.
func TestCollectorWakeIgnoresFailuresOutsideItsWorkloads(t *testing.T) {
	c := New(Env{History: strayPodWakeModel(t, kube.WakeMinWorkloads, 0)})
	c.noteWake(wakeAt.Add(-time.Minute))
	c.noteWake(wakeAt.Add(time.Hour))
	assert.Nil(t, c.wake, "no started workload had a startup failure")
}

func TestFreshWakeDropsALineLongAfterTheWakeUp(t *testing.T) {
	line := &compose.WakeLine{Started: 6, From: wakeAt,
		To: wakeAt.Add(time.Minute), Blips: 1}
	end := wakeAt.Add(time.Minute + kube.WakeQuiet)
	assert.Same(t, line, freshWake(line, end.Add(DigestWindow)))
	assert.Nil(t, freshWake(line, end.Add(wakeLineMaxDelay+time.Minute)))
	assert.Nil(t, freshWake(nil, end))
}
