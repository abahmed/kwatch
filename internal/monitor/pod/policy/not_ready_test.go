package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func notReadyPod() *corev1.Pod {
	transition := metav1.Now().Add(-2 * time.Minute)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.Time{Time: transition},
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: false,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
}

func TestNotReadyRuleAlertsAfterThreshold(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,

		Pod: notReadyPod(),
	}
	rule := NotReadyRule{Threshold: time.Minute}
	assert.Equal(t, DecisionAlert, rule.Detect(ctx))
	assert.True(t, ctx.PodHasIssues)
	assert.False(t, ctx.ContainersHasIssues)
	assert.Equal(t, constant.ReasonContainersNotReady, ctx.PodReason)
}

func TestNotReadyRuleSilentBeforeThreshold(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,

		Pod: notReadyPod(),
	}
	// Threshold greater than the time since last transition → not yet.
	rule := NotReadyRule{Threshold: 5 * time.Minute}
	assert.Equal(t, DecisionDefer, rule.Detect(ctx))
	assert.False(t, ctx.PodHasIssues)
}

func TestNotReadyRuleSilentWhenReady(t *testing.T) {
	pod := notReadyPod()
	pod.Status.Conditions[0].Status = corev1.ConditionTrue
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,

		Pod: pod,
	}
	rule := NotReadyRule{Threshold: time.Minute}
	assert.Equal(t, DecisionDefer, rule.Detect(ctx))
}

func TestNotReadyRuleSkipsCrashingContainer(t *testing.T) {
	pod := notReadyPod()
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
	}
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,

		Pod: pod,
	}
	rule := NotReadyRule{Threshold: time.Minute}
	assert.Equal(t, DecisionDefer, rule.Detect(ctx))
}

func TestNotReadyRuleSkipsNonRunningPhase(t *testing.T) {
	pod := notReadyPod()
	pod.Status.Phase = corev1.PodPending
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,

		Pod: pod,
	}
	rule := NotReadyRule{Threshold: time.Minute}
	assert.Equal(t, DecisionDefer, rule.Detect(ctx))
}

func TestNotReadyRuleSkipsWhenAlreadyAlerted(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     testNow,

		Findings: Findings{
			PodLastState: &model.ContainerState{
				Reason: constant.ReasonContainersNotReady,
			},
		},

		Pod: notReadyPod(),
	}
	rule := NotReadyRule{Threshold: time.Minute}
	assert.Equal(t, DecisionDefer, rule.Detect(ctx))
}

func TestNotReadyRuleRespectsWatchStartTime(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			WatchStartTime: time.Now(),
		}),
		Now: testNow,

		Pod: notReadyPod(),
	}
	rule := NotReadyRule{Threshold: time.Minute}
	assert.Equal(t, DecisionDefer, rule.Detect(ctx))
}

func bootingPod(
	unready time.Duration,
	everReady bool,
	probe *corev1.Probe,
) *corev1.Pod {
	pod := notReadyPod()
	pod.Status.Conditions[0].LastTransitionTime = metav1.Time{
		Time: time.Now().Add(-unready),
	}
	pod.Spec.Containers = []corev1.Container{{Name: "app", StartupProbe: probe}}
	if everReady {
		// Created an hour before it stopped being ready: it was serving in
		// between.
		pod.CreationTimestamp = metav1.Time{
			Time: time.Now().Add(-unready - time.Hour),
		}
	} else {
		// A never-ready pod's PodReady=False is stamped at creation.
		pod.CreationTimestamp = metav1.Time{Time: time.Now().Add(-unready)}
	}
	return pod
}

type detection struct {
	Status Decision
	PodMsg string
}

func detectAt(
	rule NotReadyRule,
	pod *corev1.Pod,
	kwatchUp time.Duration,
) detection {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			WatchStartTime: time.Now().Add(-kwatchUp),
		}),

		Pod: pod,
		Now: testNow,
	}
	return detection{Status: rule.Detect(ctx), PodMsg: ctx.PodMsg}
}

// The threshold is derived from the pod's own probes. A service that declares
// 150s of startup budget must not alert on every rollout at 60s.
func TestNotReadyRuleHonoursDeclaredStartupBudget(t *testing.T) {
	rule := NotReadyRule{Threshold: DefaultNotReadyThreshold}
	slow := &corev1.Probe{
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		FailureThreshold:    12,
	} // 150s

	assert.Equal(
		t,
		DecisionDefer,
		detectAt(rule, bootingPod(90*time.Second, false, slow), time.Hour).Status,
		"inside its own budget: still starting",
	)
	c := detectAt(rule, bootingPod(200*time.Second, false, slow), time.Hour)
	assert.Equal(t, DecisionAlert, c.Status, "past its own budget")
	assert.Contains(t, c.PodMsg, "never become ready")
	assert.Contains(t, c.PodMsg, "allowed 2m30s")

	assert.Equal(
		t,
		DecisionAlert,
		detectAt(rule, bootingPod(90*time.Second, false, nil), time.Hour).Status,
		"no probes declared: the 60s floor applies",
	)
}

func TestNotReadyRuleCapsStartupBudget(t *testing.T) {
	absurd := &corev1.Probe{
		InitialDelaySeconds: 0,
		PeriodSeconds:       60,
		FailureThreshold:    600,
	} // 10h
	assert.Equal(
		t,
		maxStartupBudget,
		StartupBudget(bootingPod(time.Minute, false, absurd)),
		"a probe cannot defer an alert indefinitely",
	)
}

// A pod that was ready and stopped is a regression, not a slow start. It gets
// the floor, not the startup budget, and different wording.
func TestNotReadyRuleDegradedPodDoesNotGetStartupGrace(t *testing.T) {
	rule := NotReadyRule{Threshold: DefaultNotReadyThreshold}
	slow := &corev1.Probe{
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		FailureThreshold:    12,
	}
	c := detectAt(rule, bootingPod(90*time.Second, true, slow), time.Hour)
	assert.Equal(t, DecisionAlert, c.Status)
	assert.Contains(t, c.PodMsg, "stopped being ready")
	assert.NotContains(t, c.PodMsg, "allowed")
}

// The reported duration is the truth, never the threshold. Every alert used to
// say "not ready for 1m0s" regardless of how long it had actually been.
func TestNotReadyRuleReportsRealDuration(t *testing.T) {
	rule := NotReadyRule{Threshold: DefaultNotReadyThreshold}
	c := detectAt(rule, bootingPod(3*time.Hour, true, nil), time.Hour)
	assert.Equal(t, DecisionAlert, c.Status)
	assert.Contains(t, c.PodMsg, "3h ago")
	assert.NotContains(t, c.PodMsg, "1m0s")
}

// A kwatch restart holds alerts for one threshold so every pre-existing
// condition does not fire at once — but it must not rewrite the duration.
func TestNotReadyRuleRestartGraceDoesNotFalsifyDuration(t *testing.T) {
	rule := NotReadyRule{Threshold: DefaultNotReadyThreshold}
	assert.Equal(
		t,
		DecisionDefer,
		detectAt(rule, bootingPod(3*time.Hour, true, nil), 10*time.Second).Status,
		"quiet right after kwatch starts",
	)
	c := detectAt(rule, bootingPod(3*time.Hour, true, nil), 61*time.Second)
	assert.Equal(t, DecisionAlert, c.Status)
	assert.Contains(
		t,
		c.PodMsg,
		"3h ago",
		"the grace period gates the alert, not the truth",
	)
}

// A container killed by its liveness probe before it ever passed readiness
// restarts too. That must not be mistaken for "was ready, then degraded" —
// it is the slow-start case the startup budget exists for.
func TestNotReadyRuleLivenessKilledPodIsStillStartingUp(t *testing.T) {
	rule := NotReadyRule{Threshold: DefaultNotReadyThreshold}
	slow := &corev1.Probe{
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		FailureThreshold:    12,
	} // 150s
	pod := bootingPod(90*time.Second, false, slow)
	// liveness kills, never ready
	pod.Status.ContainerStatuses[0].RestartCount = 2
	c := detectAt(rule, pod, time.Hour)
	assert.Equal(
		t,
		DecisionDefer,
		c.Status,
		"restarts alone do not prove it was ever ready",
	)
}
