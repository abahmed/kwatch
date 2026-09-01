package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestSweepControlPlaneWithLister(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	healthy := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver-healthy",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "kube-apiserver"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "apiserver",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	broken := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-scheduler-broken",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "kube-scheduler"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "scheduler",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason: "Error", ExitCode: 137,
						},
					},
				},
			},
		},
	}

	nonCP := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "random-app",
			Namespace: "default",
			Labels:    map[string]string{"app": "my-app"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Pods().Informer().GetIndexer().Add(healthy)
	f.Core().V1().Pods().Informer().GetIndexer().Add(broken)
	f.Core().V1().Pods().Informer().GetIndexer().Add(nonCP)
	h.listers.CPPod = f.Core().V1().Pods().Lister()

	h.SweepControlPlane()

	assert.Equal(
		t,
		1,
		e.ActiveCount(),
		"only the broken control-plane pod should create an incident",
	)

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "ControlPlaneComponentFailure" {
			found = true
			assert.Contains(t, v.Name, "kube-scheduler-broken")
		}
	}
	assert.True(
		t,
		found,
		"ControlPlaneComponentFailure incident should exist for broken pod",
	)
}

func TestDetectControlPlanePodPodCompletedReason(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
					Reason: "PodCompleted",
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "apiserver",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(t, sig, "PodCompleted should be skipped")
}

func TestDetectControlPlanePodSucceeded(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(t, sig, "succeeded pod should not produce a signal")
}

func TestDetectControlPlanePodRunningAndReady(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "apiserver",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(t, sig, "running and ready pod should not produce a signal")
}

func TestDetectControlPlanePodNotReadyNoContainerIssue(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
					Reason: "ContainersNotReady",
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "apiserver",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(
		t,
		sig,
		"not ready with all containers running should not produce a signal",
	)
}

func TestDetectControlPlanePodCrashLoopBackOff(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "apiserver",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig)
	assert.Equal(t, "ControlPlaneComponentFailure", sig.Reason)
	assert.Equal(t, "controlplane", sig.Resource)
	assert.Equal(t, model.SeverityHigh, sig.Severity)
	assert.Equal(t, "apiserver", sig.Container)
}

func TestDetectControlPlanePodTerminatedNonZero(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "etcd", Namespace: "kube-system"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "etcd",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig)
	assert.Equal(t, "ControlPlaneComponentFailure", sig.Reason)
}

func TestDetectControlPlanePodTerminatedZeroExit(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "etcd", Namespace: "kube-system"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "etcd",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Completed",
							ExitCode: 0,
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(
		t,
		sig,
		"container terminated with exit code 0 should not produce a signal",
	)
}

func TestDetectControlPlanePodContainerCreating(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-scheduler",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "scheduler",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ContainerCreating",
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(
		t,
		sig,
		"container with ContainerCreating should not produce a signal",
	)
}
