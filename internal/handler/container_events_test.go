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

func TestProcessContainerEventListerError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(
		client,
		&config.Config{MaxRecentLogLines: 10},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(client, 0)
	h.listers.Event = &errorEventLister{f.Core().V1().Events().Lister()}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: event lister with matching events ---

func TestProcessContainerEventListerWithEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(
		client,
		&config.Config{MaxRecentLogLines: 10},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "broken-pod",
		},
		Type:   "Warning",
		Reason: "BackOff",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.listers.Event = f.Core().V1().Events().Lister()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: k8s.GetPodEvents error ---

func TestProcessContainerGetEventsError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	client.PrependReactor(
		"list",
		"events",
		func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, assert.AnError
		},
	)
	h := NewHandler(
		client,
		&config.Config{MaxRecentLogLines: 10},
		e,
		testAlertMgr,
	)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: ContainerKillingFilter enricher skip ---

func TestProcessContainerKillingFilterSkip(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines:            10,
		IgnoreFailedGracefulShutdown: true,
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "killed-pod",
		},
		Type:    "Warning",
		Reason:  "Killing",
		Message: "Stopping container app",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.listers.Event = f.Core().V1().Events().Lister()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "killed-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 1,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "killing filter should skip container")
}

// --- executeContainersFilters: owner resolved ---

func TestProcessContainerOwnerResolved(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-rs",
			Namespace: "default",
			UID:       "rs-uid",
		},
	}
	f.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(rs)
	h.listers.RS = f.Apps().V1().ReplicaSets().Lister()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-pod",
			Namespace: "default",
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
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- buildContainerHint: OOMKilled with spec but no memory limit ---

func TestProcessContainerWithMsg(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(
		client,
		&config.Config{MaxRecentLogLines: 10},
		e,
		testAlertMgr,
	)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ErrImagePull",
							Message: "Back-off pulling image",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- container issue with container message populated (Msg path in
// buildsignalEvent) ---

func TestProcessContainerSignalEvent(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

// --- CronJob issue: invalid schedule through ProcessCronJob ---

func TestProcessContainerEventListerMultiEvents(t *testing.T) {
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
			Name: "broken-pod",
		},
		Type:          "Warning",
		Reason:        "BackOff",
		LastTimestamp: t1,
	}
	ev2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev2", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "broken-pod",
		},
		Type:          "Warning",
		Reason:        "BackOff",
		LastTimestamp: t2,
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev1)
	f.Core().V1().Events().Informer().GetIndexer().Add(ev2)
	h.listers.Event = f.Core().V1().Events().Lister()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- buildContainerHint: LivenessProbeFailed with spec ---

// The informer keeps a by-pod index of events. When it is available it must be
// used instead of listing the whole namespace and filtering on every alert.
