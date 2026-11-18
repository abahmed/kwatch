package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ruleContainerPod is a pod whose app container has rules and whose
// status shows its latest exit with code, after a run of run.
func ruleContainerPod(
	rules []corev1.ContainerRestartRule, code int32, run time.Duration,
	terminatedNow bool,
) *corev1.Pod {
	p := pod("p1")
	never := corev1.ContainerRestartPolicyNever
	p.Spec.Containers = []corev1.Container{{Name: "app", Image: "app:1",
		RestartPolicy: &never, RestartPolicyRules: rules}}
	start := metav1.NewTime(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	exit := &corev1.ContainerStateTerminated{ExitCode: code,
		Reason: "Error", StartedAt: start,
		FinishedAt: metav1.NewTime(start.Add(run))}
	status := corev1.ContainerStatus{Name: "app", RestartCount: 1}
	if terminatedNow {
		status.State.Terminated = exit
	} else {
		status.State.Waiting = &corev1.ContainerStateWaiting{
			Reason: "CrashLoopBackOff"}
		status.LastTerminationState.Terminated = exit
	}
	p.Status.ContainerStatuses = []corev1.ContainerStatus{status}
	return p
}

func exitRule(
	op corev1.ContainerRestartRuleOnExitCodesOperator, codes ...int32,
) corev1.ContainerRestartRule {
	return corev1.ContainerRestartRule{
		Action: corev1.ContainerRestartRuleActionRestart,
		ExitCodes: &corev1.ContainerRestartRuleOnExitCodes{
			Operator: op, Values: codes},
	}
}

func TestContainerRestartRuleMatchesLatestExit(t *testing.T) {
	in, notIn := corev1.ContainerRestartRuleOnExitCodesOpIn,
		corev1.ContainerRestartRuleOnExitCodesOpNotIn
	tt := []struct {
		name       string
		rules      []corev1.ContainerRestartRule
		code       int32
		terminated bool
		want       bool
	}{
		{"listed code restarts", []corev1.ContainerRestartRule{
			exitRule(in, 42)}, 42, false, true},
		{"listed code while terminated", []corev1.ContainerRestartRule{
			exitRule(in, 42)}, 42, true, true},
		{"other code is no rule restart", []corev1.ContainerRestartRule{
			exitRule(in, 42)}, 1, false, false},
		{"catch-all NotIn declares nothing", []corev1.ContainerRestartRule{
			exitRule(notIn, 0)}, 1, false, false},
		{"first matching rule decides", []corev1.ContainerRestartRule{
			exitRule(notIn, 42), exitRule(in, 42)}, 42, false, true},
		{"earlier NotIn match wins", []corev1.ContainerRestartRule{
			exitRule(notIn, 0), exitRule(in, 42)}, 42, false, false},
		{"no rules", nil, 42, false, false},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			descs := containerDescriptionsViaSchema(t, ruleContainerPod(
				tc.rules, tc.code, 90*time.Second, tc.terminated))
			attrs := descs[0].Attributes
			_, set := attrs[kube.AttrRestartRule]
			assert.Equal(t, tc.want, set)
			run, timed := attrs[kube.AttrRestartRuleRun]
			assert.Equal(t, tc.want, timed)
			if tc.want {
				seconds, _ := run.AsNumber()
				assert.InDelta(t, 90, seconds, 0.001)
			}
		})
	}
}
