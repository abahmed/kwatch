package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var testGap = Gap{
	Start: time.Date(2026, 10, 22, 2, 10, 0, 0, time.UTC),
	End:   time.Date(2026, 10, 22, 7, 45, 0, 0, time.UTC),
}

func gapModel(t *testing.T, apply ...inventory.Observation) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	for _, o := range apply {
		_, err := m.Apply(o)
		require.NoError(t, err)
	}
	return m
}

func gapNode(name string, readySince time.Time) inventory.Observation {
	return observed(inventory.CoreID(kube.KindNode, "", name),
		map[string]inventory.Value{
			kube.AttrReadySince: inventory.Time(readySince)})
}

// gapContainer is a pod on node with one container that last ended at
// finished.
func gapContainer(
	pod, node string, finished time.Time, reason string, restarts int,
) []inventory.Observation {
	podID := inventory.CoreID(kube.KindPod, "shop", pod)
	c := kube.ContainerID("shop", pod, "app")
	return []inventory.Observation{
		related(podID, inventory.RunsOn,
			inventory.CoreID(kube.KindNode, "", node)),
		related(c, inventory.PartOf, podID),
		observed(c, map[string]inventory.Value{
			kube.AttrLastFinished: inventory.Time(finished),
			kube.AttrLastReason:   inventory.Text(reason),
			kube.AttrLastExitCode: inventory.Number(137),
			kube.AttrRestarts:     inventory.Number(float64(restarts)),
		}),
	}
}

func TestGapCrashesSkipNodesThatRebootedInTheGap(t *testing.T) {
	inGap := testGap.Start.Add(time.Hour)
	obs := []inventory.Observation{
		gapNode("stable", testGap.Start.Add(-24*time.Hour)),
		gapNode("rebooted", testGap.Start.Add(2*time.Hour)),
	}
	obs = append(obs, gapContainer("api-1", "stable", inGap,
		"OOMKilled", 3)...)
	obs = append(obs, gapContainer("api-2", "rebooted", inGap,
		"Error", 1)...)
	obs = append(obs, gapContainer("api-3", "stable",
		testGap.Start.Add(-time.Minute), "Error", 1)...)

	crashes, rebooted := gapCrashes(gapModel(t, obs...), testGap)

	require.Len(t, crashes, 1, "only the crash in the gap on a stable node")
	assert.Equal(t, "api-1", crashes[0].pod.Name)
	assert.Equal(t, "OOMKilled", crashes[0].reason)
	assert.Equal(t, 3, crashes[0].restarts)
	assert.Equal(t, 1, rebooted)
}

func TestGapReportScaledDownAndBackIsNothing(t *testing.T) {
	before := modelWithDeployment(t, 3)
	saved := map[string]string{}
	for k, v := range Fingerprints(before, testGap.Start) {
		saved[k] = v.(string)
	}
	// Scaled 3 -> 0 -> 3 while kwatch was down: the end state is equal.
	after := modelWithDeployment(t, 3)

	changes := DowntimeChanges(after, saved, nil, testGap.End)

	assert.Empty(t, changes)
	assert.Equal(t, "kwatch was not running from 02:10 to 07:45 UTC "+
		"(5h35m). Compared with before, nothing changed and nothing "+
		"crashed.", gapReport(testGap, changes, nil, 0))
}

func TestGapReportListsChangesThenCrashes(t *testing.T) {
	changes := []inventory.Observation{{
		Entity: inventory.CoreID(kube.KindDeployment, "shop", "api"),
		Change: inventory.Change{Fields: []inventory.FieldChange{
			{Path: "spec.template"}}},
	}, {
		Entity: inventory.CoreID(kube.KindNode, "", "n1"),
		Change: inventory.Change{Fields: []inventory.FieldChange{
			{Path: kube.AttrTaints}}},
	}}
	crashes := []gapCrash{
		{pod: inventory.CoreID(kube.KindPod, "shop", "api-1"),
			container: "app", reason: "OOMKilled", restarts: 1},
		{pod: inventory.CoreID(kube.KindPod, "shop", "api-2"),
			container: "app", reason: "Error", exitCode: 2, restarts: 4},
	}

	got := gapReport(testGap, changes, crashes, 2)

	assert.Equal(t, "kwatch was not running from 02:10 to 07:45 UTC "+
		"(5h35m). Since then:\n\n"+
		"Changed (who is unknown, kwatch was down):\n"+
		"- Deployment shop/api: spec.template changed\n"+
		"- Node n1: taints changed\n\n"+
		"Crashed:\n"+
		"- shop/api-1, container app: last stopped by OOMKilled, "+
		"1 restart in total\n"+
		"- shop/api-2, container app: last stopped with exit code 2, "+
		"4 restarts in total\n\n"+
		"Crashes on 2 nodes that went not-ready during the gap can't be "+
		"told apart, so they are not listed.\n"+
		"Anything still broken is reported as its own incident.", got)
}

func TestGapReportCapsItems(t *testing.T) {
	var crashes []gapCrash
	for range 12 {
		crashes = append(crashes, gapCrash{
			pod:       inventory.CoreID(kube.KindPod, "shop", "p"),
			container: "app", reason: "Error", exitCode: 1, restarts: 2})
	}

	got := gapReport(testGap, nil, crashes, 0)

	assert.Contains(t, got, "\nand 2 more.\n")
	assert.NotContains(t, got, "Changed")
	assert.Equal(t, 10, strings.Count(got, "\n- shop/p"))
}

func TestBaselineSamplerPausedAfterGapLearnsNothing(t *testing.T) {
	r := newSamplerRig(t)
	r.observe(r.deploy, map[string]inventory.Value{
		kube.AttrTemplateHash: inventory.Text("v1")})
	container := r.container()
	r.sampler.pauseUntil(timelineAt(14 * time.Hour))
	r.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: container, At: timelineAt(13 * time.Hour),
		Attributes: map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(9)}})

	stat := r.stat(inventory.MetricRestartsPerHour)

	assert.Zero(t, stat.Count, "the startup storm is not learned")
}
