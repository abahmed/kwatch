package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// twoContainerPod is a running pod, not ready for ten minutes, with a
// ready main container and a helper in the given state. owner, when
// set, owns the pod.
func twoContainerPod(
	owner inventory.Kind, helper map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	pod := newID(kube.KindPod, "shop", "etl-x")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(t0),
	})
	main := newID(kube.KindContainer, "shop", "etl-x/load")
	put(m, main, t0, map[string]inventory.Value{
		kube.AttrState: inventory.Text("running"),
		kube.AttrReady: inventory.Bool(true),
	})
	link(m, main, inventory.PartOf, pod)
	side := newID(kube.KindContainer, "shop", "etl-x/fetch")
	put(m, side, t0, helper)
	link(m, side, inventory.PartOf, pod)
	if owner != "" {
		link(m, pod, inventory.OwnedBy, newID(owner, "shop", "etl"))
	}
	return m, pod
}

func finishedHelper(code float64) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrState:    inventory.Text("terminated"),
		kube.AttrReady:    inventory.Bool(false),
		kube.AttrExitCode: inventory.Number(code),
	}
}

func TestPodNotReadyOnlyByFinishedContainer(t *testing.T) {
	waiting := map[string]inventory.Value{
		kube.AttrState: inventory.Text("waiting"),
		kube.AttrReady: inventory.Bool(false),
	}
	cases := map[string]struct {
		owner  inventory.Kind
		helper map[string]inventory.Value
		want   []string
	}{
		"job pod whose helper exited 0 is as it should be": {
			kube.KindJob, finishedHelper(0), []string{}},
		"bare pod whose helper exited 0 too": {
			"", finishedHelper(0), []string{}},
		"helper that failed keeps the pod reported": {
			kube.KindJob, finishedHelper(1),
			[]string{reasons.ContainersNotReady}},
		"helper that waits keeps the pod reported": {
			kube.KindJob, waiting, []string{reasons.ContainersNotReady}},
		"a replica set restarts an exited container": {
			kube.KindReplicaSet, finishedHelper(0),
			[]string{reasons.ContainersNotReady}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, pod := twoContainerPod(tc.owner, tc.helper)
			got := evaluate(NewPod(PodThresholds{}), m,
				t0.Add(10*time.Minute), pod, nil).Findings
			assert.Equal(t, tc.want, findingReasons(got))
		})
	}
}

// A pod whose containers all stopped, none ready, is not "finished
// helpers beside a working main container".
func TestPodNotReadyWithNoReadyContainerStays(t *testing.T) {
	m, pod := twoContainerPod(kube.KindJob, finishedHelper(0))
	put(m, newID(kube.KindContainer, "shop", "etl-x/load"), t0,
		map[string]inventory.Value{
			kube.AttrState: inventory.Text("running"),
			kube.AttrReady: inventory.Bool(false),
		})
	got := evaluate(NewPod(PodThresholds{}), m, t0.Add(10*time.Minute),
		pod, nil).Findings
	assert.Equal(t, []string{reasons.ContainersNotReady},
		findingReasons(got))
}
