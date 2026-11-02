package dashboard

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const schedulerSays = "0/5 nodes are available: 3 Insufficient memory, " +
	"2 node(s) had untolerated taint {dedicated: gpu}. preemption: 0/5 " +
	"nodes are available: 5 Preemption is not helpful for scheduling."

// pendingPod is a pod of root the scheduler has not placed.
func (tm *testModel) pendingPod(
	root inventory.EntityID, name, condition string,
) inventory.EntityID {
	key := kube.ConditionKey("PodScheduled")
	attrs := map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
		key:            inventory.Text("False"),
	}
	if condition != "" {
		attrs[key+kube.AttrConditionMessage] = inventory.Text(condition)
		attrs[key+kube.AttrConditionSince] = inventory.Time(
			testEpoch.Add(-10 * time.Minute))
	}
	return tm.pod(root, name, attrs)
}

func TestDetailWaitingExplainsTheSchedulerInPlainWords(t *testing.T) {
	tm := newTestModel(t)
	root, _ := tm.deployment()
	a := tm.pendingPod(root, "api-a", "")
	tm.pendingPod(root, "api-b", "")
	tm.note(a, failedScheduling, schedulerSays, 40, time.Minute)

	d := detailOf(t, contextServer(tm.m, false,
		failingAt("s", root, testEpoch.Add(-time.Hour))), "s")
	w := d.Waiting
	if w == nil || w.Pods != 2 || w.Pod != "api-a" || w.Nodes != 5 ||
		w.Message != schedulerSays ||
		!w.At.Equal(testEpoch.Add(-time.Minute)) || len(w.Reasons) != 2 {
		t.Fatalf("waiting = %+v", w)
	}
	if w.Reasons[0] != (WaitReason{Text: "Not enough free memory",
		Reason: "Insufficient memory", Nodes: 3}) ||
		w.Reasons[1].Text != "The node has a taint the pod does not "+
			"tolerate" || w.Reasons[1].Nodes != 2 {
		t.Errorf("reasons = %+v", w.Reasons)
	}
}

func TestDetailWaitingFallsBackToTheCondition(t *testing.T) {
	tm := newTestModel(t)
	root, _ := tm.deployment()
	tm.pendingPod(root, "api-a", "0/3 nodes are available: 3 node(s) "+
		"had something new.")
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("s", root, testEpoch.Add(-time.Hour))), "s")
	w := d.Waiting
	if w == nil || w.Nodes != 3 || len(w.Reasons) != 1 ||
		w.Reasons[0].Text != "had something new" ||
		!w.At.Equal(testEpoch.Add(-10*time.Minute)) {
		t.Fatalf("waiting = %+v", w)
	}
}

func TestDetailWaitingAbsentForPlacedPods(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	tm.observe(pod, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running")})
	tm.note(pod, failedScheduling, schedulerSays, 1, time.Hour)
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("s", root, testEpoch.Add(-time.Hour))), "s")
	if d.Waiting != nil {
		t.Fatalf("a running pod waits: %+v", d.Waiting)
	}
}

func TestWaitReasonsAreBounded(t *testing.T) {
	msg := "0/90 nodes are available: 10 a1, 10 a2, 10 a3, 10 a4, " +
		"10 a5, 10 a6, 10 a7, 10 a8, 10 a9."
	got, total, cut := waitReasons(msg)
	if len(got) != maxWaitReasons || total != 90 || cut != 1 {
		t.Fatalf("reasons = %+v, total %d, cut %d", got, total, cut)
	}
}

func TestBlockerTextKnowsTheSchedulerWords(t *testing.T) {
	for reason, want := range map[string]string{
		"Insufficient cpu": "Not enough free CPU",
		"Insufficient nvidia.com/gpu": "Not enough of a resource it " +
			"asks for",
		"didn't match Pod's node affinity/selector": "The pod's node " +
			"selector or node affinity does not match the node",
		"had volume node affinity conflict": "The pod's volume is in " +
			"another zone",
		"Too many pods":      "The node already runs as many pods as it allows",
		"were unschedulable": "The node is cordoned",
	} {
		if got := blockerText(reason); got != want {
			t.Errorf("%s: %q, want %q", reason, got, want)
		}
	}
}
