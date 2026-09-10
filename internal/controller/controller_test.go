package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func newTestController(
	t testing.TB,
	client kubernetes.Interface,
	cfg *config.Config,
	h handler.Handler,
) (*Controller, func()) {
	t.Helper()
	ctrl, cleanup, err := New(client, cfg, h)
	require.NoError(t, err)
	return ctrl, cleanup
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
func (m *mockHandler) SetListers(handler.Listers)       {}
func (m *mockHandler) SetClock(func() time.Time)        {}
func (m *mockHandler) Owners() observe.OwnerResolver    { return nil }
func (m *mockHandler) SetNamespaceScope([]string, bool) {}
func (m *mockHandler) SweepTLSSecrets()                 {}
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
func (m *mockHandler) ReportStartupSummary(suppressed map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startupSummary = suppressed
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

func TestNewWiresEndpointSlicesForAdmissionWebhookMonitor(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AdmissionWebhookMonitor: config.AdmissionWebhookMonitor{Enabled: true},
	}

	ctrl, cleanup := newTestController(t, client, cfg, &mockHandler{})
	defer cleanup()

	assert.NotNil(t, ctrl.endpointSliceLister)
	assert.NotEmpty(t, ctrl.endpointSlice.synced)
	assert.False(t, ctrl.endpointSlice.startWorkers)
}

func TestEndpointSliceDeleteRequeuesServiceFromTombstone(t *testing.T) {
	ctrl := &Controller{
		service:       newResourcePipeline("service", "services"),
		endpointSlice: newResourcePipeline("endpointslice", "endpointslices"),
	}
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-hash",
			Namespace: "ns",
			Labels: map[string]string{
				endpointSliceServiceLabel: "web",
			},
		},
	}
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "ns/web-hash",
		Obj: epSlice,
	}

	ctrl.endpointSliceEventHandler(true).DeleteFunc(tombstone)
	item, shutdown := ctrl.service.queue.Get()
	defer ctrl.service.queue.Done(item)
	defer ctrl.service.queue.ShutDown()

	assert.False(t, shutdown)
	assert.Equal(t, "ns/web", item)
}

func TestEndpointSliceEventRequeuesMatchingWebhooks(t *testing.T) {
	client := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(client, 0)
	mwcInformer := factory.Admissionregistration().V1().
		MutatingWebhookConfigurations()
	vwcInformer := factory.Admissionregistration().V1().
		ValidatingWebhookConfigurations()
	serviceRef := &admissionregistrationv1.ServiceReference{
		Namespace: "ns",
		Name:      "web",
	}
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: serviceRef,
			},
		}},
	}
	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: serviceRef,
			},
		}},
	}
	require.NoError(t, mwcInformer.Informer().GetIndexer().Add(mwc))
	require.NoError(t, vwcInformer.Informer().GetIndexer().Add(vwc))

	ctrl := &Controller{
		mwc:       newResourcePipeline("mwc", "mwc"),
		vwc:       newResourcePipeline("vwc", "vwc"),
		mwcLister: mwcInformer.Lister(),
		vwcLister: vwcInformer.Lister(),
	}
	ctrl.mwc.startWorkers = true
	ctrl.vwc.startWorkers = true
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-hash",
			Namespace: "ns",
			Labels: map[string]string{
				endpointSliceServiceLabel: "web",
			},
		},
	}

	ctrl.endpointSliceEventHandler(false).AddFunc(epSlice)

	mwcItem, mwcShutdown := ctrl.mwc.queue.Get()
	vwcItem, vwcShutdown := ctrl.vwc.queue.Get()
	defer ctrl.mwc.queue.Done(mwcItem)
	defer ctrl.vwc.queue.Done(vwcItem)
	defer ctrl.mwc.queue.ShutDown()
	defer ctrl.vwc.queue.ShutDown()

	assert.False(t, mwcShutdown)
	assert.False(t, vwcShutdown)
	assert.Equal(t, "mwc", mwcItem)
	assert.Equal(t, "vwc", vwcItem)
}

func TestNewWithSingleNamespace(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		AllowedNamespaces: []string{"production"},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.NotNil(ctrl)
	assert.NotNil(ctrl.podLister)
}

func TestSyncEndpointSliceResolvesServiceByLabel(t *testing.T) {
	assert := assert.New(t)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Selector:  map[string]string{"app": "web"},
		},
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
	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	// The slice name ("web-hash") must NOT be looked up as the service name.
	err := ctrl.syncEndpointSlice(context.Background(), "ns/web-hash")
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
	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	err := ctrl.syncEndpointSlice(context.Background(), "ns/web-hash")
	assert.Nil(err)
}
