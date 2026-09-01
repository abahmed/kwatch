package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNewWithResync(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ResyncSeconds: 300,
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.podLister)
}

func TestEnqueuePod(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		pod: newResourcePipeline("pod", "pods"),
	}
	defer ctrl.pod.shutdown()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pod",
			Namespace: "default",
		},
	}

	ctrl.pod.enqueue(pod)
	assert.Equal(1, ctrl.pod.queue.Len())

	key, quit := ctrl.pod.queue.Get()
	assert.False(quit)
	assert.Equal("default/my-pod", key)
	ctrl.pod.queue.Done(key)
}

func TestEnqueueNode(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		node: newResourcePipeline("node", "nodes"),
	}
	defer ctrl.node.shutdown()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
	}

	ctrl.node.enqueue(node)
	assert.Equal(1, ctrl.node.queue.Len())

	key, quit := ctrl.node.queue.Get()
	assert.False(quit)
	assert.Equal("worker-1", key)
	ctrl.node.queue.Done(key)
}

func TestEnqueueNodeTombstone(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		node: newResourcePipeline("node", "nodes"),
	}
	defer ctrl.node.shutdown()

	tombstone := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-2",
		},
	}
	ctrl.node.enqueue(tombstone)
	assert.Equal(1, ctrl.node.queue.Len())

	key, _ := ctrl.node.queue.Get()
	assert.Equal("worker-2", key)
	ctrl.node.queue.Done(key)
}

func TestProcessNextPodItemQuit(t *testing.T) {
	assert := assert.New(t)

	h := &mockHandler{}
	ctrl := &Controller{
		pod:     newResourcePipeline("pod", "pods"),
		handler: h,
	}

	ctrl.pod.queue.ShutDown()
	result := ctrl.pod.processNextItem(context.Background())
	assert.False(result)
	assert.Empty(h.podKeys)
}

func TestProcessNextNodeItemQuit(t *testing.T) {
	assert := assert.New(t)

	h := &mockHandler{}
	ctrl := &Controller{
		node:    newResourcePipeline("node", "nodes"),
		handler: h,
	}

	ctrl.node.queue.ShutDown()
	result := ctrl.node.processNextItem(context.Background())
	assert.False(result)
	assert.Empty(h.nodeKeys)
}

func TestProcessNextPodItemProcessesKey(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	ctrl, cleanup := newTestController(t, client, &config.Config{}, h)
	defer cleanup()

	ctrl.pod.queue.Add("default/test-pod")
	result := ctrl.pod.processNextItem(context.Background())
	assert.True(result)
	assert.Equal([]string{"default/test-pod"}, h.podKeys)
	assert.Equal([]bool{false}, h.podDel)
}

func TestProcessNextNodeItemProcessesKey(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	ctrl.node.queue.Add("worker-1")
	result := ctrl.node.processNextItem(context.Background())
	assert.True(result)
	assert.Equal([]string{"worker-1"}, h.nodeKeys)
	assert.Equal([]bool{false}, h.nodeDel)
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

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(func() bool {
		_, err := ctrl.podLister.Pods("default").Get("my-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	err := ctrl.syncPod(context.Background(), "default/my-pod")
	assert.NoError(err)
	assert.Equal([]string{"default/my-pod"}, h.podKeys)
	assert.Equal([]bool{false}, h.podDel)
}

func TestSyncPodDeletedPod(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	err := ctrl.syncPod(context.Background(), "default/nonexistent")
	assert.NoError(err)
	assert.Equal([]string{"default/nonexistent"}, h.podKeys)
	assert.Equal([]bool{false}, h.podDel)
}

func TestSyncPodInvalidKey(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	// The key is forwarded to the handler, which is responsible for parsing
	// and validating it; the controller is a thin dispatch layer.
	err := ctrl.syncPod(
		context.Background(),
		"invalid-key-without-namespace/extra/segments",
	)
	assert.NoError(err)
	assert.Equal(
		[]string{"invalid-key-without-namespace/extra/segments"},
		h.podKeys,
	)
	assert.Equal([]bool{false}, h.podDel)
}

func TestSyncPodHandlerError(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{err: errors.New("handler failed")}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	err := ctrl.syncPod(context.Background(), "default/nonexistent")
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

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(func() bool {
		_, err := ctrl.nodeLister.Get("worker-1")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	err := ctrl.syncNode(context.Background(), "worker-1")
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

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	err := ctrl.syncNode(context.Background(), "nonexistent-node")
	assert.NoError(err)
	assert.Equal([]string{"nonexistent-node"}, h.nodeKeys)
	assert.Equal([]bool{false}, h.nodeDel)
}

func TestSyncNodeHandlerError(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{err: errors.New("node handler failed")}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	err := ctrl.syncNode(context.Background(), "nonexistent-node")
	assert.Error(err)
	assert.Equal("node handler failed", err.Error())
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
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

	ctrl, cleanup := newTestController(t, client, cfg, h)
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
