package handler

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSweepControlPlaneWithLister(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

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
				{Name: "apiserver", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
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
				{Name: "app", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Core().V1().Pods().Informer().GetIndexer().Add(healthy)
	f.Core().V1().Pods().Informer().GetIndexer().Add(broken)
	f.Core().V1().Pods().Informer().GetIndexer().Add(nonCP)
	h.SetCpPodLister(f.Core().V1().Pods().Lister())

	h.SweepControlPlane()

	assert.Equal(t, 1, e.ActiveCount(), "only the broken control-plane pod should create an incident")

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "ControlPlaneComponentFailure" {
			found = true
			assert.Contains(t, v.Name, "kube-scheduler-broken")
		}
	}
	assert.True(t, found, "ControlPlaneComponentFailure incident should exist for broken pod")
}

func TestDetectControlPlanePodSucceeded(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-apiserver", Namespace: "kube-system"},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(t, sig, "succeeded pod should not produce a signal")
}

func TestDetectControlPlanePodRunningAndReady(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-apiserver", Namespace: "kube-system"},
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

func TestDetectControlPlanePodCrashLoopBackOff(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-apiserver", Namespace: "kube-system"},
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
	assert.Equal(t, "high", sig.Severity)
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
	assert.Nil(t, sig, "container terminated with exit code 0 should not produce a signal")
}

func TestDetectControlPlanePodContainerCreating(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-scheduler", Namespace: "kube-system"},
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
	assert.Nil(t, sig, "container with ContainerCreating should not produce a signal")
}

func TestDetectControlPlanePodPodInitializing(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-scheduler", Namespace: "kube-system"},
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
							Reason: "PodInitializing",
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(t, sig, "container with PodInitializing should not produce a signal")
}

func TestDetectControlPlanePodUnschedulable(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-controller-manager", Namespace: "kube-system"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/4 nodes are available",
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig)
	assert.Equal(t, "ControlPlaneComponentFailure", sig.Reason)
	assert.Contains(t, sig.Hint, "Unschedulable")
}

func TestDetectControlPlanePodInitContainerError(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "init-sysctl",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 1,
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig, "init container error should produce a signal")
	assert.Equal(t, "init-sysctl", sig.Container)
}

func TestProcessControlPlanePodHealthy(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
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

	assert.NoError(t, h.ProcessControlPlanePod(pod))
	assert.Equal(t, 0, e.ActiveCount(), "healthy control-plane pod should not create incident")
}

func TestProcessControlPlanePodBrokenCreatesIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
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

	assert.NoError(t, h.ProcessControlPlanePod(pod))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "ControlPlaneComponentFailure" {
			found = true
			assert.Equal(t, model.StateActive, v.State)
		}
	}
	assert.True(t, found, "ControlPlaneComponentFailure incident should exist")
}

func TestProcessControlPlanePodNonControlPlane(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	pod := &corev1.Pod{
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

	assert.NoError(t, h.ProcessControlPlanePod(pod))
	assert.Equal(t, 0, e.ActiveCount(), "non-control-plane pod should not be processed")
}

func TestComponentNameFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{"nil labels", nil, ""},
		{"empty labels", map[string]string{}, ""},
		{"kube-apiserver", map[string]string{"component": "kube-apiserver"}, "kube-apiserver"},
		{"kube-scheduler", map[string]string{"component": "kube-scheduler"}, "kube-scheduler"},
		{"kube-controller-manager", map[string]string{"component": "kube-controller-manager"}, "kube-controller-manager"},
		{"etcd", map[string]string{"component": "etcd"}, "etcd"},
		{"kube-proxy", map[string]string{"k8s-app": "kube-proxy"}, "kube-proxy"},
		{"coredns", map[string]string{"k8s-app": "kube-dns"}, "coredns"},
		{"metrics-server", map[string]string{"k8s-app": "metrics-server"}, "metrics-server"},
		{"unknown", map[string]string{"component": "unknown"}, ""},
		{"wrong label key", map[string]string{"app": "kube-apiserver"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComponentNameFromLabels(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSweepControlPlaneNoLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SweepControlPlane() // should not panic
}
