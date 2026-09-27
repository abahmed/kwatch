package policy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPodStatusRuleReportsAdmissionRejectedPod(t *testing.T) {
	ctx := &Context{
		EvType: "ADDED",
		Pod: &corev1.Pod{Status: corev1.PodStatus{
			Phase:   corev1.PodFailed,
			Reason:  "OutOfcpu",
			Message: "Pod was rejected: Node didn't have enough resource",
		}},
	}
	if got := (PodStatusRule{}).Detect(ctx); got != DecisionAlert {
		t.Fatalf("decision = %v, want alert", got)
	}
	if !ctx.PodHasIssues || ctx.PodReason != "OutOfcpu" {
		t.Fatalf("pod issue = %v reason = %q", ctx.PodHasIssues, ctx.PodReason)
	}
}
