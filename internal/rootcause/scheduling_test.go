package rootcause

import "testing"

func TestParseSchedulerMessageIgnoresPreemptionVerdict(t *testing.T) {
	msg := "0/5 nodes are available: 2 Insufficient cpu, 3 node(s) " +
		"didn't match Pod's node affinity/selector. preemption: 0/5 " +
		"nodes are available: 5 Preemption is not helpful for " +
		"scheduling."

	blockers, total := ParseSchedulerMessage(msg)

	if total != 5 || len(blockers) != 2 {
		t.Fatalf("total=%d blockers=%+v", total, blockers)
	}
	if blockers[0].Reason != "didn't match Pod's node affinity/selector" ||
		blockers[0].Nodes != 3 {
		t.Errorf("top = %+v", blockers[0])
	}
	if blockers[1].Reason != "Insufficient cpu" || blockers[1].Nodes != 2 {
		t.Errorf("second = %+v", blockers[1])
	}
}

func TestParseSchedulerMessageOnlyPreemptionHasNoBlockers(t *testing.T) {
	msg := "0/3 nodes are available: preemption: 0/3 nodes are " +
		"available: 3 Preemption is not helpful for scheduling."
	blockers, total := ParseSchedulerMessage(msg)
	if total != 3 || len(blockers) != 0 {
		t.Errorf("total=%d blockers=%+v", total, blockers)
	}
}
