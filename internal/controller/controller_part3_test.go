package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestRunEndToEndPodDelete(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, 1)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ephemeral",
			Namespace: "default",
		},
	}
	_, err := client.CoreV1().Pods(
		"default",
	).Create(
		ctx,
		pod,
		metav1.CreateOptions{},
	)
	assert.NoError(err)

	assert.Eventually(func() bool {
		return h.podCount() > 0
	}, 5*time.Second, 100*time.Millisecond)

	// Reset tracking by appending a separator
	h.mu.Lock()
	h.podKeys = nil
	h.podDel = nil
	h.mu.Unlock()

	err = client.CoreV1().Pods(
		"default",
	).Delete(
		ctx,
		"ephemeral",
		metav1.DeleteOptions{},
	)
	assert.NoError(err)

	assert.Eventually(func() bool {
		return h.podCount() > 0
	}, 5*time.Second, 100*time.Millisecond)

	key, del := h.podEntry(0)
	assert.Equal("default/ephemeral", key)
	assert.False(del)
}

func TestRunEndToEndNodeAdd(t *testing.T) {
	assert := assert.New(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
	}
	client := fake.NewSimpleClientset(node)
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, 1)

	assert.Eventually(func() bool {
		return h.nodeCount() > 0
	}, 5*time.Second, 100*time.Millisecond)

	key, del := h.nodeEntry(0)
	assert.Equal("worker-1", key)
	assert.False(del)
}

func TestRunEndToEndRequeueOnError(t *testing.T) {
	assert := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retry-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	client := fake.NewSimpleClientset(pod)
	cfg := &config.Config{}
	h := &mockHandler{err: errors.New("transient error")}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, 1)

	// Handler returns error — pod should be requeued and processed again
	assert.Eventually(func() bool {
		return h.podCount() >= 2
	}, 5*time.Second, 100*time.Millisecond)

	key0, _ := h.podEntry(0)
	key1, _ := h.podEntry(1)
	assert.Equal("default/retry-pod", key0)
	assert.Equal("default/retry-pod", key1)
}

func TestRunPodDeduplication(t *testing.T) {
	assert := assert.New(t)

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[string](),
	)
	defer q.ShutDown()

	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	ctrl := &Controller{
		handler:   &mockHandler{},
		podLister: f.Core().V1().Pods().Lister(),
	}
	ctrl.pod = newResourcePipeline("pod", "pods")
	ctrl.pod.queue = q
	ctrl.pod.syncFn = ctrl.syncPod

	q.Add("default/dup")
	q.Add("default/dup")

	assert.Equal(1, q.Len())

	assert.True(ctrl.pod.processNextItem(context.Background()))

	q.ShutDown()
	assert.False(ctrl.pod.processNextItem(context.Background()))

	assert.Equal(1, ctrl.handler.(*mockHandler).podCount())
}

func TestMultipleWorkers(t *testing.T) {
	assert := assert.New(t)

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[string](),
	)
	defer q.ShutDown()

	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	ctrl := &Controller{
		handler:   &mockHandler{},
		podLister: f.Core().V1().Pods().Lister(),
	}
	ctrl.pod = newResourcePipeline("pod", "pods")
	ctrl.pod.queue = q
	ctrl.pod.syncFn = ctrl.syncPod

	for i := 0; i < 10; i++ {
		q.Add(fmt.Sprintf("default/pod-%d", i))
	}

	for i := 0; i < 10; i++ {
		ctrl.pod.processNextItem(context.Background())
	}

	assert.Equal(10, ctrl.handler.(*mockHandler).podCount())
	assert.Equal(0, q.Len())
}

func TestEnqueuePodWithTombstone(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		pod: newResourcePipeline("pod", "pods"),
	}
	defer ctrl.pod.shutdown()

	tombstone := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tombstone-pod",
			Namespace: "kube-system",
		},
	}
	ctrl.pod.enqueue(tombstone)
	assert.Equal(1, ctrl.pod.queue.Len())

	key, _ := ctrl.pod.queue.Get()
	assert.Equal("kube-system/tombstone-pod", key)
	ctrl.pod.queue.Done(key)
}

func TestEnqueuePodDeletedFinalStateUnknown(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		pod: newResourcePipeline("pod", "pods"),
	}
	defer ctrl.pod.shutdown()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lost-pod",
			Namespace: "default",
		},
	}
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/lost-pod",
		Obj: pod,
	}
	ctrl.pod.enqueue(tombstone)
	assert.Equal(1, ctrl.pod.queue.Len())

	key, _ := ctrl.pod.queue.Get()
	assert.Equal("default/lost-pod", key)
	ctrl.pod.queue.Done(key)
}

func TestProcessNextPodItemForgetsOnSuccess(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	ctrl, cleanup := newTestController(t, client, &config.Config{}, h)
	defer cleanup()

	ctrl.pod.queue.Add("default/forgotten")

	ctrl.pod.processNextItem(context.Background())

	assert.Equal(0, ctrl.pod.queue.Len())
}

func TestNewMultiNamespaceHasMultipleSynced(t *testing.T) {
	assert := assert.New(t)

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "ns2"},
	}
	client := fake.NewSimpleClientset(pod1, pod2)
	cfg := &config.Config{
		AllowedNamespaces: []string{"ns1", "ns2"},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Len(ctrl.pod.synced, 2, "should have one synced fn per namespace")
}

func TestRunMultipleWorkers(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, 4)

	// Add 20 pods via the pod queue
	for i := 0; i < 20; i++ {
		ctrl.pod.queue.Add(fmt.Sprintf("default/pod-%d", i))
	}

	assert.Eventually(func() bool {
		return h.podCount() >= 20
	}, 10*time.Second, 100*time.Millisecond, "all 20 pods should be processed "+
		"with 4 workers")

	cancel()
}

func TestMultiNamespaceListerSeesBothNamespaces(t *testing.T) {
	assert := assert.New(t)

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "ns2"},
	}
	client := fake.NewSimpleClientset(pod1, pod2)
	cfg := &config.Config{
		AllowedNamespaces: []string{"ns1", "ns2"},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(func() bool {
		_, err1 := ctrl.podLister.Pods("ns1").Get("pod-1")
		_, err2 := ctrl.podLister.Pods("ns2").Get("pod-2")
		return err1 == nil && err2 == nil
	}, 5*time.Second, 50*time.Millisecond)
}

func TestBuildSeenSetSeedsNodeConditions(t *testing.T) {
	assert := assert.New(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}
	client := fake.NewSimpleClientset(node)
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(func() bool {
		_, err := ctrl.nodeLister.Get("worker-1")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	// Node conditions SHOULD be seeded into baseline (BASE-1b)
	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	expectedKey := correlation.BuildKey("", "worker-1", "MemoryPressure", "")
	assert.Contains(
		baseline,
		string(expectedKey),
		"buildSeenSet must seed node conditions",
	)
}

// errDaemonSetLister is a DaemonSetLister whose List always fails,
// simulating a broken informer cache.
type errDaemonSetLister struct {
	appsv1lister.DaemonSetLister
}
