package controller

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func newTestController(
	t testing.TB,
	client kubernetes.Interface,
	cfg *config.Config,
	h *mockHandler,
) (*Controller, func()) {
	t.Helper()
	ctrl, cleanup, err := NewWithRuntimeConfig(
		client, config.RuntimeConfigFor(cfg), componentsFor(h),
		RuntimeDependencies{Now: clock.RealClock{}.Now},
	)
	require.NoError(t, err)
	return ctrl, cleanup
}

func newTestChangeTracker(capacity int) *kwcontext.ChangeTracker {
	return kwcontext.NewChangeTrackerWithClock(
		capacity, clock.RealClock{},
	)
}

func componentsFor(h *mockHandler) RuntimeSet {
	return RuntimeSet{
		Pod: PodRuntime{
			Processor: h,
		},
		Node: NodeRuntime{
			Processor: h,
		},
		Workload: WorkloadRuntime{
			Deployments:  h,
			DaemonSets:   h,
			StatefulSets: h,
			CronJobs:     h,
			HPAs:         h,
			PDBs:         h,
			ReplicaSets:  h,
			Jobs:         h,
		},
		Network: NetworkRuntime{
			Processor: h,
		},
		Security: SecurityRuntime{
			Processor: h,
		},
		Cluster: ClusterRuntime{
			Processor: h,
		},
		Integration: IntegrationRuntime{
			ControlPlane: h,
			Events:       h,
		},
		Baseline: h,
	}
}

type mockHandler struct {
	mu             sync.Mutex
	podKeys        []string
	podDel         []bool
	nodeKeys       []string
	nodeDel        []bool
	err            error
	seenBaseline   map[string]map[string]int64
	activeNodes    []string
	startupSummary map[string]int
}

func (m *mockHandler) ProcessPod(
	_ context.Context,
	key string,
	deleted bool,
) error {
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

func (m *mockHandler) ProcessDeployment(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessReplicaSet(string, bool) error {
	return m.err
}

func (m *mockHandler) ProcessJob(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessDaemonSet(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessCronJob(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessHorizontalPodAutoscaler(
	string,
	bool,
) error {
	return m.err
}
func (m *mockHandler) Owners() observe.OwnerResolver    { return nil }
func (m *mockHandler) SetNamespaceScope([]string, bool) {}
func (m *mockHandler) SweepTLSSecrets() error           { return nil }
func (m *mockHandler) SetBaseline(baseline map[string]map[string]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seenBaseline = baseline
}
func (m *mockHandler) SetActiveNodeIncidents(nodes []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeNodes = append(m.activeNodes, nodes...)
}
func (m *mockHandler) ClearBaselineForPod(
	string, string, model.ObjectRef,
) {
}
func (m *mockHandler) ProcessMutatingWebhookConfiguration(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessValidatingWebhookConfiguration(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessService(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessNetworkPolicy(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessIngress(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessResourceQuota(string, bool) error {
	return m.err
}

func (m *mockHandler) ProcessLimitRange(string, bool) error {
	return m.err
}

func (m *mockHandler) ProcessNamespace(string, bool) error {
	return m.err
}

func (m *mockHandler) ProcessLease(string, bool) error {
	return m.err
}

func (m *mockHandler) ProcessControlPlanePod(
	*corev1.Pod,
) error {
	return m.err
}

func (m *mockHandler) SweepControlPlane() {}

func (m *mockHandler) ProcessStatefulSet(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessPdb(
	string,
	bool,
) error {
	return m.err
}

func (m *mockHandler) ProcessNodeResourceOvercommit(
	string,
	string,
	string,
	model.Severity,
) {
}

func (m *mockHandler) ProcessClusterAutoscalerEvent(
	*corev1.Event,
) {
}

func (m *mockHandler) ProcessWarningEvent(*corev1.Event) {}

func TestNewCreatesController(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.pod.queue)
	assert.NotNil(ctrl.node.queue)
	assert.NotNil(ctrl.podLister)
	assert.Len(ctrl.pod.synced, 1)
	// Node monitor disabled by default — no node informer
	assert.Nil(ctrl.node.synced)
	assert.Nil(ctrl.nodeLister)
}

func TestNewWithNodeMonitor(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.NotEmpty(ctrl.node.synced)
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

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	// Node resource monitoring needs the node lister even when the node
	// event monitor is disabled — a nil lister would panic on first tick.
	assert.NotNil(ctrl.nodeLister)
	// But the node event worker must stay off.
	assert.Nil(ctrl.node.synced)
}
