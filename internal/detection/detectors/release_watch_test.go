package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// releaseCase describes one Deployment rollout for the table.
type releaseCase struct {
	// history is the restart count of each earlier hour.
	history []float64
	// restarts is the restarts of each of the two new pods.
	restarts [2]float64
	// age is how long ago the new pods were created.
	age time.Duration
	// oldRevision is false for a first deploy: no earlier ReplicaSet.
	firstDeploy bool
	crashLoop   bool
	// lastRestart is how long ago the last restart ended; zero is unset.
	lastRestart time.Duration
}

func buildRelease(t *testing.T, c releaseCase) (
	*inventory.Model, inventory.EntityID,
) {
	t.Helper()
	m := newTestModel()
	dep := newID(kube.KindDeployment, "shop", "api")
	put(m, dep, t0.Add(-2*time.Hour), map[string]inventory.Value{
		kube.AttrReplicas: inventory.Number(2),
	})
	revisions := []struct {
		name, revision string
	}{{"api-old", "13"}, {"api-new", "14"}}
	if c.firstDeploy {
		revisions = revisions[1:]
	}
	for _, r := range revisions {
		rs := newID(kube.KindReplicaSet, "shop", r.name)
		put(m, rs, t0.Add(-2*time.Hour), map[string]inventory.Value{
			kube.AttrRevision: inventory.Text(r.revision),
		})
		link(m, rs, inventory.OwnedBy, dep)
	}
	rs := newID(kube.KindReplicaSet, "shop", "api-new")
	for i, restarts := range c.restarts {
		addReleasePod(m, rs, i, t0.Add(-c.age), restarts, c)
	}
	for hour, count := range c.history {
		at := t0.Add(-time.Duration(len(c.history)+2-hour) * time.Hour)
		m.Baselines().AddRestarts(dep, int(count), at)
	}
	if len(c.history) > 0 {
		m.Baselines().AddRestarts(dep, 0, t0.Add(-3*time.Hour/2))
	}
	return m, dep
}

func addReleasePod(
	m *inventory.Model, rs inventory.EntityID, i int, created time.Time,
	restarts float64, c releaseCase,
) {
	pod := newID(kube.KindPod, "shop", "api-new-"+string(rune('a'+i)))
	put(m, pod, created, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
	})
	link(m, pod, inventory.OwnedBy, rs)
	attrs := map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(restarts),
	}
	if c.lastRestart > 0 {
		attrs[kube.AttrLastFinished] = inventory.Time(t0.Add(-c.lastRestart))
	}
	if c.crashLoop {
		attrs[kube.AttrStateReason] = inventory.Text(reasons.CrashLoopBackOff)
	}
	container := newID(kube.KindContainer, "shop", pod.Name+"/app")
	put(m, container, created, attrs)
	link(m, container, inventory.PartOf, pod)
}

func TestReleaseWatch(t *testing.T) {
	steady := make([]float64, 12)
	steady[3] = 2
	flappy := []float64{4, 3, 5, 4, 4, 3, 5, 4, 4, 3, 5, 4}
	busy := []float64{20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20}
	tests := []struct {
		name string
		c    releaseCase
		want bool
	}{
		{"worse release fires", releaseCase{
			history: steady, restarts: [2]float64{3, 4},
			age: 10 * time.Minute}, true},
		{"previous had no restarts at all fires", releaseCase{
			restarts: [2]float64{2, 2}, age: 10 * time.Minute}, true},
		{"similar release is quiet", releaseCase{
			history: flappy, restarts: [2]float64{1, 1},
			age: 10 * time.Minute}, false},
		{"same high rate is quiet", releaseCase{
			history: busy, restarts: [2]float64{3, 3},
			age: 10 * time.Minute}, false},
		{"first deploy is quiet", releaseCase{
			firstDeploy: true, restarts: [2]float64{3, 4},
			age: 10 * time.Minute}, false},
		{"outside the window is quiet", releaseCase{
			history: steady, restarts: [2]float64{3, 4},
			age: 20 * time.Minute}, false},
		{"window ended, restarts continue keeps the finding", releaseCase{
			history: steady, restarts: [2]float64{3, 4},
			age: 20 * time.Minute, lastRestart: 2 * time.Minute}, true},
		{"window ended, quiet for 10 minutes clears", releaseCase{
			history: steady, restarts: [2]float64{3, 4},
			age: 20 * time.Minute, lastRestart: 10 * time.Minute}, false},
		{"tiny numbers are quiet", releaseCase{
			history: steady, restarts: [2]float64{1, 1},
			age: 10 * time.Minute}, false},
		{"crash loop is left to the container finding", releaseCase{
			history: steady, restarts: [2]float64{3, 4},
			age: 10 * time.Minute, crashLoop: true}, false},
		{"high restart count is left to the container finding",
			releaseCase{history: steady, restarts: [2]float64{6, 0},
				age: 10 * time.Minute}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, dep := buildRelease(t, tt.c)
			got := evaluate(ReleaseWatch{}, m, t0, dep, nil).Findings
			if !tt.want {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, reasons.ReleaseRegression, got[0].Reason)
		})
	}
}

func TestReleaseWatchSummaryAndRecheck(t *testing.T) {
	steady := make([]float64, 12)
	steady[3] = 2
	m, dep := buildRelease(t, releaseCase{
		history: steady, restarts: [2]float64{3, 4}, age: 10 * time.Minute,
	})
	eval := evaluate(ReleaseWatch{}, m, t0, dep, nil)
	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Contains(t, f.Summary, "Rollout 14 restarted 7 times")
	assert.Contains(t, f.Summary, "times as often as rollout 13 did")
	assert.Equal(t, time.Minute, eval.RecheckAfter,
		"a watched release is looked at again")
	labels := map[string]string{}
	for _, e := range f.Evidence {
		labels[e.Label] = e.Value
	}
	assert.Equal(t, "14", labels["new revision"])
	assert.Equal(t, "13", labels["previous revision"])
}

func TestReleaseWatchWithoutRestartHistory(t *testing.T) {
	m, dep := buildRelease(t, releaseCase{
		restarts: [2]float64{2, 2}, age: 10 * time.Minute,
	})
	got := evaluate(ReleaseWatch{}, m, t0, dep, nil).Findings
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "rollout 13 did not restart")
}
