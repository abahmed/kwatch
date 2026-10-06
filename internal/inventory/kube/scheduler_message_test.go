package kube

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

func TestParseSchedulerMessageKeepsDotsInsideReasons(t *testing.T) {
	tests := []struct {
		name, msg, reason string
		nodes             int
	}{
		{"extended resource",
			"0/4 nodes are available: 4 Insufficient nvidia.com/gpu.",
			"Insufficient nvidia.com/gpu", 4},
		{"dotted taint key",
			"0/3 nodes are available: 3 node(s) had untolerated taint " +
				"{node-role.kubernetes.io/control-plane: }.",
			"had untolerated taint", 3},
		{"volume node affinity",
			"0/3 nodes are available: 3 node(s) had volume node " +
				"affinity conflict.",
			"had volume node affinity conflict", 3},
	}
	for _, test := range tests {
		blockers, _ := ParseSchedulerMessage(test.msg)
		if len(blockers) != 1 || blockers[0].Reason != test.reason ||
			blockers[0].Nodes != test.nodes {
			t.Errorf("%s: blockers=%+v", test.name, blockers)
		}
	}
}

func TestParseSchedulerMessageSplitsOnSeparatorsOnly(t *testing.T) {
	msg := "0/5 nodes are available: 2 Insufficient example.com/fpga, " +
		"3 node(s) had untolerated taint {dedicated.io/gpu: true}."
	blockers, total := ParseSchedulerMessage(msg)
	if total != 5 || len(blockers) != 2 {
		t.Fatalf("total=%d blockers=%+v", total, blockers)
	}
	if blockers[1].Reason != "Insufficient example.com/fpga" {
		t.Errorf("second = %+v", blockers[1])
	}
}
