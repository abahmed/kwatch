package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/util/workqueue"
)

type mockHandler struct {
	mu       sync.Mutex
	podKeys  []string
	podDel   []bool
	nodeKeys []string
	nodeDel  []bool
	err      error
}

func (m *mockHandler) ProcessPod(key string, deleted bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.podKeys = append(m.podKeys, key)
	m.podDel = append(m.podDel, deleted)
	return m.err
}
func (m *mockHandler) ProcessNode(key string, deleted bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeKeys = append(m.nodeKeys, key)
	m.nodeDel = append(m.nodeDel, deleted)
	return m.err
}
func (m *mockHandler) podCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.podKeys)
}
func (m *mockHandler) nodeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodeKeys)
}
func (m *mockHandler) podEntry(i int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.podKeys[i], m.podDel[i]
}
func (m *mockHandler) nodeEntry(i int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodeKeys[i], m.nodeDel[i]
}
func (m *mockHandler) ProcessPodObject(*corev1.Pod, bool) error   { return m.err }
func (m *mockHandler) ProcessNodeObject(*corev1.Node, bool) error { return m.err }
func (m *mockHandler) SetPodLister(corev1lister.PodLister)        {}
func (m *mockHandler) SetNodeLister(corev1lister.NodeLister)      {}

func TestNewCreatesController(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.podQueue)
	assert.NotNil(ctrl.nodeQueue)
	assert.NotNil(ctrl.podLister)
	assert.NotNil(ctrl.podsSynced)
	// Node monitor disabled by default — no node informer
	assert.Nil(ctrl.nodesSynced)
	assert.Nil(ctrl.nodeLister)
}

func TestNewWithNodeMonitor(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl.nodesSynced)
	assert.NotNil(ctrl.nodeLister)
}

func TestNewWithSingleNamespace(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AllowedNamespaces: []string{"production"},
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.podLister)
}

func TestNewWithResync(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ResyncSeconds: 300,
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.podLister)
}

func TestEnqueuePod(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		podQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
	}
	defer ctrl.podQueue.ShutDown()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pod",
			Namespace: "default",
		},
	}

	ctrl.enqueuePod(pod)
	assert.Equal(1, ctrl.podQueue.Len())

	key, quit := ctrl.podQueue.Get()
	assert.False(quit)
	assert.Equal("default/my-pod", key)
	ctrl.podQueue.Done(key)
}

func TestEnqueueNode(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		nodeQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
	}
	defer ctrl.nodeQueue.ShutDown()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
	}

	ctrl.enqueueNode(node)
	assert.Equal(1, ctrl.nodeQueue.Len())

	key, quit := ctrl.nodeQueue.Get()
	assert.False(quit)
	assert.Equal("worker-1", key)
	ctrl.nodeQueue.Done(key)
}

func TestEnqueueNodeTombstone(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		nodeQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
	}
	defer ctrl.nodeQueue.ShutDown()

	tombstone := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-2",
		},
	}
	ctrl.enqueueNode(tombstone)
	assert.Equal(1, ctrl.nodeQueue.Len())

	key, _ := ctrl.nodeQueue.Get()
	assert.Equal("worker-2", key)
	ctrl.nodeQueue.Done(key)
}

func TestProcessNextPodItemQuit(t *testing.T) {
	assert := assert.New(t)

	h := &mockHandler{}
	ctrl := &Controller{
		podQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		handler: h,
	}

	ctrl.podQueue.ShutDown()
	result := ctrl.processNextPodItem()
	assert.False(result)
	assert.Empty(h.podKeys)
}

func TestProcessNextNodeItemQuit(t *testing.T) {
	assert := assert.New(t)

	h := &mockHandler{}
	ctrl := &Controller{
		nodeQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		handler: h,
	}

	ctrl.nodeQueue.ShutDown()
	result := ctrl.processNextNodeItem()
	assert.False(result)
	assert.Empty(h.nodeKeys)
}

func TestProcessNextPodItemProcessesKey(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	ctrl, cleanup := New(client, &config.Config{}, h)
	defer cleanup()

	ctrl.podQueue.Add("default/test-pod")
	result := ctrl.processNextPodItem()
	assert.True(result)
	assert.Equal([]string{"default/test-pod"}, h.podKeys)
	assert.Equal([]bool{true}, h.podDel)
}

func TestProcessNextNodeItemProcessesKey(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	ctrl.nodeQueue.Add("worker-1")
	result := ctrl.processNextNodeItem()
	assert.True(result)
	assert.Equal([]string{"worker-1"}, h.nodeKeys)
	assert.Equal([]bool{true}, h.nodeDel)
}

func TestSyncPodExistingPod(t *testing.T) {
	assert := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pod",
			Namespace: "default",
		},
	}
	client := fake.NewSimpleClientset(pod)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Eventually(func() bool {
		_, err := ctrl.podLister.Pods("default").Get("my-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	err := ctrl.syncPod("default/my-pod")
	assert.NoError(err)
	assert.Equal([]string{"default/my-pod"}, h.podKeys)
	assert.Equal([]bool{false}, h.podDel)
}

func TestSyncPodDeletedPod(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	err := ctrl.syncPod("default/nonexistent")
	assert.NoError(err)
	assert.Equal([]string{"default/nonexistent"}, h.podKeys)
	assert.Equal([]bool{true}, h.podDel)
}

func TestSyncPodInvalidKey(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	err := ctrl.syncPod("invalid-key-without-namespace/extra/segments")
	assert.Error(err)
	assert.Empty(h.podKeys)
}

func TestSyncPodHandlerError(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{err: errors.New("handler failed")}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	err := ctrl.syncPod("default/nonexistent")
	assert.Error(err)
	assert.Equal("handler failed", err.Error())
}

func TestSyncNodeExistingNode(t *testing.T) {
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

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Eventually(func() bool {
		_, err := ctrl.nodeLister.Get("worker-1")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	err := ctrl.syncNode("worker-1")
	assert.NoError(err)
	assert.Equal([]string{"worker-1"}, h.nodeKeys)
	assert.Equal([]bool{false}, h.nodeDel)
}

func TestSyncNodeDeletedNode(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	err := ctrl.syncNode("nonexistent-node")
	assert.NoError(err)
	assert.Equal([]string{"nonexistent-node"}, h.nodeKeys)
	assert.Equal([]bool{true}, h.nodeDel)
}

func TestSyncNodeHandlerError(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{err: errors.New("node handler failed")}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	err := ctrl.syncNode("nonexistent-node")
	assert.Error(err)
	assert.Equal("node handler failed", err.Error())
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ctrl.Run(ctx, 1)
	}()

	time.Sleep(200 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		assert.NoError(err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

func TestRunEndToEndPodAdd(t *testing.T) {
	assert := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	client := fake.NewSimpleClientset(pod)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, 1)

	assert.Eventually(func() bool {
		return h.podCount() > 0
	}, 5*time.Second, 100*time.Millisecond)

	key, del := h.podEntry(0)
	assert.Equal("default/app-pod", key)
	assert.False(del)
}

func TestRunEndToEndPodDelete(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
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
	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	assert.NoError(err)

	assert.Eventually(func() bool {
		return h.podCount() > 0
	}, 5*time.Second, 100*time.Millisecond)

	// Reset tracking by appending a separator
	h.mu.Lock()
	h.podKeys = nil
	h.podDel = nil
	h.mu.Unlock()

	err = client.CoreV1().Pods("default").Delete(ctx, "ephemeral", metav1.DeleteOptions{})
	assert.NoError(err)

	assert.Eventually(func() bool {
		return h.podCount() > 0
	}, 5*time.Second, 100*time.Millisecond)

	key, del := h.podEntry(0)
	assert.Equal("default/ephemeral", key)
	assert.True(del)
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

	ctrl, cleanup := New(client, cfg, h)
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

	ctrl, cleanup := New(client, cfg, h)
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
		podQueue:  q,
		handler:   &mockHandler{},
		podLister: f.Core().V1().Pods().Lister(),
	}

	q.Add("default/dup")
	q.Add("default/dup")

	assert.Equal(1, q.Len())

	assert.True(ctrl.processNextPodItem())

	q.ShutDown()
	assert.False(ctrl.processNextPodItem())

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
		podQueue:  q,
		handler:   &mockHandler{},
		podLister: f.Core().V1().Pods().Lister(),
	}

	for i := 0; i < 10; i++ {
		q.Add(fmt.Sprintf("default/pod-%d", i))
	}

	for i := 0; i < 10; i++ {
		ctrl.processNextPodItem()
	}

	assert.Equal(10, ctrl.handler.(*mockHandler).podCount())
	assert.Equal(0, q.Len())
}

func TestEnqueuePodWithTombstone(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		podQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
	}
	defer ctrl.podQueue.ShutDown()

	tombstone := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tombstone-pod",
			Namespace: "kube-system",
		},
	}
	ctrl.enqueuePod(tombstone)
	assert.Equal(1, ctrl.podQueue.Len())

	key, _ := ctrl.podQueue.Get()
	assert.Equal("kube-system/tombstone-pod", key)
	ctrl.podQueue.Done(key)
}

func TestProcessNextPodItemForgetsOnSuccess(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	ctrl, cleanup := New(client, &config.Config{}, h)
	defer cleanup()

	ctrl.podQueue.Add("default/forgotten")

	ctrl.processNextPodItem()

	assert.Equal(0, ctrl.podQueue.Len())
}
