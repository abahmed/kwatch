package metricsapi

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

var podMetricsGVR = schema.GroupVersionResource{
	Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods",
}

type Monitor struct {
	metrics      dynamic.NamespaceableResourceInterface
	client       kubernetes.Interface
	incidentSink monitor.ObservationSink
	cfg          config.RuntimeMetricsMonitor
	allowed      func(string) bool
	namespaces   []string
	watchAll     bool
	// owners resolves a pod to the workload its incidents are keyed by.
	owners observe.OwnerResolver
	// podLister reads the controller's pod informer cache instead of listing
	// every pod from the API server on each sweep.
	podLister  corev1lister.PodLister
	configured bool
	started    bool
	mu         sync.Mutex
}

// NewWithClient constructs the monitor from the shared dynamic client.
func NewWithClient(
	dynamicClient dynamic.Interface,
	client kubernetes.Interface,
	cfg config.RuntimeMetricsMonitor,
	incidentSink monitor.ObservationSink,
) *Monitor {
	return &Monitor{
		metrics: dynamicClient.Resource(podMetricsGVR), client: client,
		incidentSink: incidentSink, cfg: cfg, watchAll: true,
	}
}

// podOwner is the incident owner for a pod, resolved the way every other
// producer resolves it.
func (m *Monitor) podOwner(pod *corev1.Pod) model.ObjectRef {
	if m.owners != nil {
		if owner := m.owners.OwnerOf(pod); owner.Name != "" {
			return owner
		}
	}
	if len(pod.OwnerReferences) == 0 {
		return observe.SelfOwner("Pod", pod.Namespace, pod.Name)
	}
	// Unresolved: keep the key this monitor has always used here. No Kind,
	// because the pod does have owner references -- kwatch just could not
	// follow them, and "Pod" would state the opposite.
	return model.ObjectRef{Name: pod.Namespace + "/" + pod.Name}
}
