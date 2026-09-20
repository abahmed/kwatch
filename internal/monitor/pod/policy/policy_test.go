package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
)

func TestNamespacePolicySuppressesForbiddenNamespace(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			ForbiddenNamespaces: []string{"staging"},
		}),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "staging"},
		},
	}

	assert.Equal(t, DecisionSuppress, (NamespaceRule{}).Detect(ctx))
}

func TestPendingPolicyUsesInjectedClock(t *testing.T) {
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Now:     func() time.Time { return started.Add(6 * time.Minute) },
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: started},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	}

	decision := (PendingPodRule{Threshold: 5 * time.Minute}).Detect(ctx)
	assert.Equal(t, DecisionAlert, decision)
	assert.Equal(t, "PodPending", ctx.PodReason)
}

func TestContainerIssueReasonPreservesBaselineIdentity(t *testing.T) {
	status := &corev1.ContainerStatus{
		Name: "api",
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: constant.ReasonCrashLoopBackOff,
			},
		},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason:  constant.ReasonOOMKilled,
				Message: "out of memory",
			},
		},
	}

	assert.Equal(t, constant.ReasonOOMKilled, ContainerIssueReason(status))
	assert.Equal(t, "out of memory", ContainerIssueMessage(status))
}
