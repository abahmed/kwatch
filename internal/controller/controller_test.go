package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

type mockHandler struct {
	mu             sync.Mutex
	podKeys        []string
	podDel         []bool
	nodeKeys       []string
	nodeDel        []bool
	err            error
	seenBaseline   map[string]map[string]int64
	startupSummary map[string]int
}

func (m *mockHandler) ProcessPod(_ context.Context, key string, deleted bool) error {
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
func (m *mockHandler) ProcessDeployment(string, bool) error                  { return m.err }
func (m *mockHandler) ProcessJob(string, bool) error                         { return m.err }
func (m *mockHandler) SetPodLister(corev1lister.PodLister)                   {}
func (m *mockHandler) SetNodeLister(corev1lister.NodeLister)                 {}
func (m *mockHandler) SetDeploymentLister(appsv1lister.DeploymentLister)     {}
func (m *mockHandler) SetJobLister(batchv1lister.JobLister)                  {}
func (m *mockHandler) SetReplicaLister(appsv1lister.ReplicaSetLister)        {}
func (m *mockHandler) SetDaemonSetLister(appsv1lister.DaemonSetLister)       {}
func (m *mockHandler) SetStatefulSetLister(appsv1lister.StatefulSetLister)   {}
func (m *mockHandler) SetPdbLister(policyv1lister.PodDisruptionBudgetLister) {}
func (m *mockHandler) SetEventLister(corev1lister.EventLister)               {}
func (m *mockHandler) ProcessDaemonSet(string, bool) error                   { return m.err }
func (m *mockHandler) ProcessCronJob(string, bool) error                     { return m.err }
func (m *mockHandler) SetCronJobLister(batchv1lister.CronJobLister)          {}
func (m *mockHandler) SetHorizontalPodAutoscalerLister(autoscalingv2lister.HorizontalPodAutoscalerLister) {
}
func (m *mockHandler) ProcessHorizontalPodAutoscaler(string, bool) error { return m.err }
func (m *mockHandler) SetSecretLister(corev1lister.SecretLister)         {}
func (m *mockHandler) SweepTLSSecrets()                                  {}
func (m *mockHandler) SetBaseline(baseline map[string]map[string]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seenBaseline = baseline
}
func (m *mockHandler) SetActiveNodeIncidents([]string)    {}
func (m *mockHandler) ClearBaselineForPod(string, string) {}
func (m *mockHandler) ReportStartupSummary(suppressed map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startupSummary = suppressed
}
func (m *mockHandler) ProcessMutatingWebhookConfiguration(string, bool) error   { return m.err }
func (m *mockHandler) ProcessValidatingWebhookConfiguration(string, bool) error { return m.err }
func (m *mockHandler) ProcessService(string, bool) error                        { return m.err }
func (m *mockHandler) ProcessNetworkPolicy(string, bool) error                  { return m.err }
func (m *mockHandler) ProcessIngress(string, bool) error                        { return m.err }
func (m *mockHandler) ProcessControlPlanePod(*corev1.Pod) error                 { return m.err }
func (m *mockHandler) SweepControlPlane()                                       {}
func (m *mockHandler) SetServiceLister(corev1lister.ServiceLister)              {}
func (m *mockHandler) SetEndpointSliceLister(discoveryv1lister.EndpointSliceLister) {
}
func (m *mockHandler) SetMwCLister(admissionregistrationv1lister.MutatingWebhookConfigurationLister) {
}
func (m *mockHandler) SetVwCLister(admissionregistrationv1lister.ValidatingWebhookConfigurationLister) {
}
func (m *mockHandler) SetIngressLister(networkingv1lister.IngressLister)                    {}
func (m *mockHandler) SetNetpolLister(networkingv1lister.NetworkPolicyLister)               {}
func (m *mockHandler) SetCpPodLister(corev1lister.PodLister)                                {}
func (m *mockHandler) ProcessStatefulSet(string, bool) error                                { return m.err }
func (m *mockHandler) ProcessPdb(string, bool) error                                        { return m.err }
func (m *mockHandler) SetInsightEngine(_ *insight.Engine)                                   {}
func (m *mockHandler) ProcessNodeResourceOvercommit(string, string, string, model.Severity) {}
func (m *mockHandler) ProcessClusterAutoscalerEvent(*corev1.Event)                          {}

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
	assert.Len(ctrl.podsSynced, 1)
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

func TestNewWithNodeResourceMonitorOnly(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		NodeResourceMonitor: config.NodeResourceMonitor{
			Enabled:         true,
			IntervalSeconds: 60,
		},
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	// Node resource monitoring needs the node lister even when the node
	// event monitor is disabled — a nil lister would panic on first tick.
	assert.NotNil(ctrl.nodeLister)
	// But the node event worker must stay off.
	assert.Nil(ctrl.nodesSynced)
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

func TestSyncEndpointSliceResolvesServiceByLabel(t *testing.T) {
	assert := assert.New(t)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.1", Selector: map[string]string{"app": "web"}},
	}
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-hash",
			Namespace: "ns",
			Labels:    map[string]string{"kubernetes.io/service-name": "web"},
		},
	}
	client := fake.NewSimpleClientset(svc, epSlice)
	cfg := &config.Config{
		ServiceMonitor: config.ServiceMonitor{Enabled: true},
	}
	h := &mockHandler{}
	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	// The slice name ("web-hash") must NOT be looked up as the service name.
	err := ctrl.syncEndpointSlice("ns/web-hash")
	assert.Nil(err)
}

func TestSyncEndpointSliceIgnoresUnlabeled(t *testing.T) {
	assert := assert.New(t)

	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "web-hash", Namespace: "ns"},
	}
	client := fake.NewSimpleClientset(epSlice)
	cfg := &config.Config{
		ServiceMonitor: config.ServiceMonitor{Enabled: true},
	}
	h := &mockHandler{}
	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	err := ctrl.syncEndpointSlice("ns/web-hash")
	assert.Nil(err)
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
	result := ctrl.processNextPodItem(context.Background())
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
	result := ctrl.processNextPodItem(context.Background())
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
	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	ctrl.nodeQueue.Add("worker-1")
	result := ctrl.processNextNodeItem()
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

	ctrl, cleanup := New(client, cfg, h)
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

	ctrl, cleanup := New(client, cfg, h)
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

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	// The key is forwarded to the handler, which is responsible for parsing
	// and validating it; the controller is a thin dispatch layer.
	err := ctrl.syncPod(context.Background(), "invalid-key-without-namespace/extra/segments")
	assert.NoError(err)
	assert.Equal([]string{"invalid-key-without-namespace/extra/segments"}, h.podKeys)
	assert.Equal([]bool{false}, h.podDel)
}

func TestSyncPodHandlerError(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{err: errors.New("handler failed")}

	ctrl, cleanup := New(client, cfg, h)
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
	assert.Equal([]bool{false}, h.nodeDel)
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

	assert.True(ctrl.processNextPodItem(context.Background()))

	q.ShutDown()
	assert.False(ctrl.processNextPodItem(context.Background()))

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
		ctrl.processNextPodItem(context.Background())
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

func TestEnqueuePodDeletedFinalStateUnknown(t *testing.T) {
	assert := assert.New(t)

	ctrl := &Controller{
		podQueue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
	}
	defer ctrl.podQueue.ShutDown()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lost-pod",
			Namespace: "default",
		},
	}
	tombstone := cache.DeletedFinalStateUnknown{Key: "default/lost-pod", Obj: pod}
	ctrl.enqueuePod(tombstone)
	assert.Equal(1, ctrl.podQueue.Len())

	key, _ := ctrl.podQueue.Get()
	assert.Equal("default/lost-pod", key)
	ctrl.podQueue.Done(key)
}

func TestProcessNextPodItemForgetsOnSuccess(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	h := &mockHandler{}
	ctrl, cleanup := New(client, &config.Config{}, h)
	defer cleanup()

	ctrl.podQueue.Add("default/forgotten")

	ctrl.processNextPodItem(context.Background())

	assert.Equal(0, ctrl.podQueue.Len())
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

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Len(ctrl.podsSynced, 2, "should have one synced fn per namespace")
}

func TestRunMultipleWorkers(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, 4)

	// Add 20 pods via the pod queue
	for i := 0; i < 20; i++ {
		ctrl.podQueue.Add(fmt.Sprintf("default/pod-%d", i))
	}

	assert.Eventually(func() bool {
		return h.podCount() >= 20
	}, 10*time.Second, 100*time.Millisecond, "all 20 pods should be processed with 4 workers")

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

	ctrl, cleanup := New(client, cfg, h)
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

	ctrl, cleanup := New(client, cfg, h)
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
	assert.Contains(baseline, string(expectedKey), "buildSeenSet must seed node conditions")
}

// errDaemonSetLister is a DaemonSetLister whose List always fails,
// simulating a broken informer cache.
type errDaemonSetLister struct {
	appsv1lister.DaemonSetLister
}

func (e *errDaemonSetLister) List(selector labels.Selector) ([]*appsv1.DaemonSet, error) {
	return nil, errors.New("cache not synced")
}

// lockedBuffer is a goroutine-safe writer used to capture klog output.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestBuildSeenSetSurfacesListerErrors(t *testing.T) {
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

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.nodeLister.Get("worker-1")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	// Simulate a broken informer cache for DaemonSets: the List error
	// must be surfaced via logging, not silently swallowed.
	ctrl.dsLister = &errDaemonSetLister{}

	var buf lockedBuffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	klog.SetOutputBySeverity("WARNING", &buf)
	klog.SetOutputBySeverity("ERROR", &buf)
	defer func() {
		klog.LogToStderr(true)
		klog.SetOutput(os.Stderr)
		klog.SetOutputBySeverity("WARNING", nil)
		klog.SetOutputBySeverity("ERROR", nil)
	}()

	ctrl.buildSeenSet()

	out := buf.String()
	assert.Contains(t, out, "failed to list daemonsets for baseline seeding")
	assert.Contains(t, out, "cache not synced")

	// A failing lister must not prevent other categories from seeding.
	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()
	expectedKey := correlation.BuildKey("", "worker-1", "MemoryPressure", "")
	assert.Contains(t, baseline, string(expectedKey), "other baseline categories must still be seeded")
}

func TestBuildSeenPerPodAndHealthySiblingKeepsBaseline(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-pod", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "c", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "failed-pod", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "c", State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "Error"}}}},
			},
		},
	)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.podLister.Pods("default").Get("healthy-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	// The failed pod's key should be baselined
	key := correlation.BuildKey("default", "dep", "Error", "")
	_, ok := baseline[string(key)]["failed-pod"]
	assert.True(t, ok, "failed pod must be baselined")

	// The healthy pod should NOT be in the baseline
	assert.NotContains(t, baseline[string(key)], "healthy-pod", "healthy pod must NOT be baselined")

	// Simulate ClearBaselineForPod for the healthy pod — should NOT affect the failed pod's entry
	h.ClearBaselineForPod("default", "healthy-pod")

	_, ok = baseline[string(key)]["failed-pod"]
	assert.True(t, ok, "ClearBaselineForPod for healthy sibling must not clear failed pod's baseline")
}

func TestBuildSeenCrashLoopHighFreq(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "cl-pod", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"}}},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app", RestartCount: 7,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}},
			},
		},
	)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.podLister.Pods("default").Get("cl-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	// Key should be CrashLoopHighFrequency (not CrashLoopBackOff) because restarts > 5
	key := correlation.BuildKey("default", "dep", "CrashLoopHighFrequency", "")
	_, ok := baseline[string(key)]["cl-pod"]
	assert.True(t, ok, "buildSeenSet must use CrashLoopHighFrequency for restarts > 5")
}

func TestBuildSeenRunningWithRestarts(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "restarted-pod", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:         "app",
					RestartCount: 3,
					State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "Error"},
					},
				}},
			},
		},
	)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.podLister.Pods("default").Get("restarted-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	// Must use LastTerminationState.Terminated.Reason ("Error"), not skip the pod
	key := correlation.BuildKey("default", "dep", "Error", "")
	_, ok := baseline[string(key)]["restarted-pod"]
	assert.True(t, ok, "Running container with restarts must be baselined using LastTerminationState.Reason")
}

func TestBuildSeenSetReportsStartupSummary(t *testing.T) {
	a := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "broken-rs"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ImagePullBackOff",
						},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(pod,
		// Need a ReplicaSet for owner resolution
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "broken-rs",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "broken-deploy"},
				},
			},
		},
	)

	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.podLister.Pods("default").Get("broken-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	a.Eventually(func() bool {
		_, err := ctrl.rsLister.ReplicaSets("default").Get("broken-rs")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	// Must have called ReportStartupSummary with non-empty suppressed counts
	h.mu.Lock()
	summary := h.startupSummary
	h.mu.Unlock()

	a.NotNil(summary, "ReportStartupSummary should have been called")
	a.Greater(len(summary), 0, "suppressed map should have entries for broken pods")

	// Verify the suppressed key format: owner/reason
	found := false
	for key, count := range summary {
		if count > 0 {
			found = true
			a.Contains(key, "/", "suppressed key should use owner/reason format")
		}
	}
	a.True(found, "at least one suppressed entry should exist")
}

func TestBuildSeenSeedsDaemonSetBaselineWithEmptyKey(t *testing.T) {
	a := assert.New(t)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			NumberAvailable:        2,
		},
	}
	client := fake.NewSimpleClientset(ds)
	cfg := &config.Config{
		DaemonSetMonitor: config.DaemonSetMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.dsLister.DaemonSets("default").Get("test-ds")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	key := correlation.BuildKey("default", "default/test-ds", "DaemonSetUnavailable", "")
	a.Contains(baseline, string(key), "buildSeenSet must seed DaemonSet issues into baseline")

	_, hasEmpty := baseline[string(key)][""]
	a.True(hasEmpty, "controller resource baseline must map under empty pod key")
}

func TestBuildSeenSeedsDeploymentUnavailableBaseline(t *testing.T) {
	a := assert.New(t)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-dep",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:            3,
			UnavailableReplicas: 1,
			ObservedGeneration:  1,
		},
	}
	client := fake.NewSimpleClientset(deploy)
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.deployLister.Deployments("default").Get("test-dep")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	key := correlation.BuildKey("default", "default/test-dep", "DeploymentUnavailable", "")
	a.Contains(baseline, string(key), "buildSeenSet must seed DeploymentUnavailable issues into baseline")

	_, hasEmpty := baseline[string(key)][""]
	a.True(hasEmpty, "deployment resource baseline must map under empty pod key")
}

func TestBuildSeenSetReportsEmptySummaryOnNoBrokenPods(t *testing.T) {
	a := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
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

	client := fake.NewSimpleClientset(pod)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := New(client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.podLister.Pods("default").Get("healthy-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	summary := h.startupSummary
	h.mu.Unlock()

	// Must be empty or nil (no broken pods to suppress)
	a.Empty(summary, "no broken pods means empty startup summary")
}
