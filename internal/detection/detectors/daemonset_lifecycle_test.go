package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func daemonMessage(node string) string {
	return "Found failed daemon pod kube-system/aws-node-x on node " +
		node + ", will try to kill it"
}

// daemonRig is a DaemonSet with one settled healthy node, one node that
// is being replaced and one that just joined.
func daemonRig(now time.Time) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	ds := newID(kube.KindDaemonSet, "kube-system", "aws-node")
	put(m, ds, now.Add(-24*time.Hour), nil)
	node := func(
		name string, age time.Duration, extra map[string]inventory.Value,
	) {
		attrs := map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(now.Add(-age)),
			kube.AttrReady:   inventory.Bool(true),
		}
		for k, v := range extra {
			attrs[k] = v
		}
		put(m, newID(kube.KindNode, "", name), now.Add(-age), attrs)
	}
	node("settled", 3*time.Hour, nil)
	node("leaving", 3*time.Hour, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	node("notready", 3*time.Hour, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	node("fresh", 2*time.Minute, nil)
	return m, ds
}

func daemonFindings(m *inventory.Model, ds inventory.EntityID,
	now time.Time,
) []string {
	return findingReasons(Event{}.Detect(testDetectorContext(m, now),
		entityOf(m, ds)))
}

// A DaemonSet pod that fails on a node that is leaving, not ready, gone
// or just joined is the node's lifecycle, not the DaemonSet's problem.
func TestFailedDaemonPodOnANodeInTransitionIsQuiet(t *testing.T) {
	now := t0.Add(3 * time.Hour)
	for _, node := range []string{"leaving", "notready", "fresh", "gone"} {
		t.Run(node, func(t *testing.T) {
			m, ds := daemonRig(now)
			noteEntity(m, ds, "FailedDaemonPod", daemonMessage(node), 1,
				now.Add(-time.Minute))
			assert.Empty(t, daemonFindings(m, ds, now))
		})
	}
}

// The young node's event is quoted again once the node is old, if the
// DaemonSet pod still fails there.
func TestFailedDaemonPodOnAFreshNodeCountsOnceItIsSettled(t *testing.T) {
	now := t0.Add(3 * time.Hour)
	m, ds := daemonRig(now)
	noteEntity(m, ds, "FailedDaemonPod", daemonMessage("fresh"), 1,
		now.Add(-time.Minute))

	assert.Contains(t, daemonFindings(m, ds, now.Add(kube.BootWindow)),
		"FailedDaemonPod")
}

// The twin: the same failure on a settled, healthy node is reported.
func TestFailedDaemonPodOnASettledNodeIsReported(t *testing.T) {
	now := t0.Add(3 * time.Hour)
	m, ds := daemonRig(now)
	noteEntity(m, ds, "FailedDaemonPod", daemonMessage("settled"), 1,
		now.Add(-time.Minute))

	assert.Contains(t, daemonFindings(m, ds, now), "FailedDaemonPod")
}

// Without a node in the message the DaemonSet's own unready pods decide.
func TestFailedDaemonPodWithoutNodeNameUsesItsPods(t *testing.T) {
	now := t0.Add(3 * time.Hour)
	for node, want := range map[string]bool{"leaving": false, "settled": true} {
		m, ds := daemonRig(now)
		pod := newID(kube.KindPod, "kube-system", "aws-node-"+node)
		put(m, pod, now.Add(-time.Hour), map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)})
		relateEntity(m, pod, inventory.OwnedBy, ds)
		relateEntity(m, pod, inventory.RunsOn, newID(kube.KindNode, "", node))
		noteEntity(m, ds, "FailedDaemonPod", "daemon pod failed", 1,
			now.Add(-time.Minute))

		got := daemonFindings(m, ds, now)
		if want {
			assert.Contains(t, got, "FailedDaemonPod", node)
		} else {
			assert.Empty(t, got, node)
		}
	}
}

// A DaemonSet whose only missing pods sit on nodes in transition is not
// unavailable; a missing pod on a settled node still is.
func TestDaemonSetUnavailableIgnoresNodesInTransition(t *testing.T) {
	now := t0.Add(3 * time.Hour)
	for node, want := range map[string]bool{"leaving": false, "settled": true} {
		m, ds := daemonRig(now)
		put(m, ds, now.Add(-time.Hour), map[string]inventory.Value{
			kube.AttrDesiredReplicas: inventory.Number(2),
			kube.AttrReadyReplicas:   inventory.Number(1)})
		pod := newID(kube.KindPod, "kube-system", "aws-node-"+node)
		put(m, pod, now.Add(-time.Hour), map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)})
		relateEntity(m, pod, inventory.OwnedBy, ds)
		relateEntity(m, pod, inventory.RunsOn, newID(kube.KindNode, "", node))

		got := evaluate(NewWorkload(0), m, now.Add(time.Hour), ds, nil).Findings
		assert.Equal(t, want, len(findingReasons(got)) > 0, node)
	}
}
