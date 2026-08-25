package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/filter"
)

func TestProcessPodListerSuccess(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	f.Core().V1().Pods().Informer().GetIndexer().Add(pod)
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)
	h.SetPodLister(f.Core().V1().Pods().Lister())
	assert.NoError(t, h.ProcessPod(context.Background(), "ns1/p1", false))
}

// --- ProcessServiceObject endpoint slice lister error ---

func TestProcessPodSignalEvent(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- ProcessIngressObject with serviceLister set (no service found safe path) ---

func TestEmitHighRestartAlertOwnerEmpty(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ContainerRestartThreshold: 1,
		MaxRecentLogLines:         10,
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restart-pod",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs", UID: "rs-uid"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "empty owner should suppress high restart alert")
}

// --- executePodFilters: StatusContinue from PendingPodFilter ---

func TestProcessPodStatusContinue(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 60},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executePodFilters: !ctx.PodHasIssues return after enrichment ---

func TestProcessPodNoIssuesAfterEnrich(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:    "Warning",
		Reason:  "FailedScheduling",
		Message: "deleting pod for maintenance",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "deleting event should clear PodHasIssues")
}

// --- executePodFilters: sort.Slice in event lister path with 2 events ---

func TestProcessPodNoIssuesAfterPodEnricher(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 600},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)
	hh := h
	hh.podEnrichers = []filter.Enricher{clearPodIssuesEnricher{}}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: ""},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}
	assert.NoError(t, hh.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount())
}
