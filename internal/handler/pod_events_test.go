package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/config"
)

func TestProcessPodEventListerWithEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:   "Warning",
		Reason: "FailedScheduling",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
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

// --- executePodFilters: event lister error ---

func TestProcessPodEventListerError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	h.SetEventLister(&errorEventLister{f.Core().V1().Events().Lister()})

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

// --- executePodFilters: k8s.GetPodEvents error ---

func TestProcessPodGetEventsError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "events", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
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

// --- executePodFilters: PodEventsFilter enricher skip (deleting event) ---

func TestProcessPodEventsDeletingPod(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "deleting-pod",
		},
		Type:    "Warning",
		Reason:  "FailedScheduling",
		Message: "deleting pod for some reason",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "deleting-pod", Namespace: "default"},
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
	assert.Equal(t, 0, e.ActiveCount(), "deleting event should suppress alert")
}

// --- executePodFilters: owner resolved ---

func TestProcessPodOwnerResolved(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rs", Namespace: "default", UID: "rs-uid"},
	}
	f.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(rs)
	h.SetReplicaLister(f.Apps().V1().ReplicaSets().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs", UID: "rs-uid"},
			},
		},
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

// --- executePodFilters: Unschedulable with ScheduleMonitor ---

func TestProcessPodEventListerMultiEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	now := metav1.Now()
	t1 := metav1.NewTime(now.Time.Add(-10 * time.Second))
	t2 := metav1.NewTime(now.Time.Add(-5 * time.Second))
	ev1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:          "Warning",
		Reason:        "FailedScheduling",
		LastTimestamp: t1,
	}
	ev2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev2", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:          "Warning",
		Reason:        "FailedScheduling",
		LastTimestamp: t2,
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev1)
	f.Core().V1().Events().Informer().GetIndexer().Add(ev2)
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
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: sort.Slice in event lister path with 2 events ---
