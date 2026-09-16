package controlplane

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

const (
	defaultInterval        = 30 * time.Second
	defaultProbeTimeout    = 10 * time.Second
	defaultFailureSamples  = 2
	defaultRecoverySamples = 2
)

type Monitor struct {
	client       kubernetes.Interface
	restClient   rest.Interface
	cfg          config.ControlPlaneMonitor
	incidentSink monitor.ObservationSink
	mu           sync.RWMutex
	status       Status
	failures     map[string]int
	recoveries   map[string]int
	failing      map[string]bool
	now          func() time.Time
	podLister    corev1lister.PodLister
	resolver     HostResolver
	configured   bool
	started      bool
}

// HostResolver is the small DNS dependency required by the CoreDNS probe.
type HostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

// NewWithRESTDependencies constructs the monitor with explicit dependencies.
func NewWithRESTDependencies(
	restClient rest.Interface,
	client kubernetes.Interface,
	cfg config.ControlPlaneMonitor,
	incidentSink monitor.ObservationSink,
	resolver HostResolver,
	timeSource clock.Clock,
) *Monitor {
	timeSource = clock.Require(timeSource)
	return &Monitor{
		client:       client,
		restClient:   restClient,
		cfg:          cfg,
		incidentSink: incidentSink,
		status:       Status{Components: make(map[string]EndpointStatus)},
		failures:     make(map[string]int),
		recoveries:   make(map[string]int),
		failing:      make(map[string]bool),
		now:          timeSource.Now,
		resolver:     resolver,
	}
}

// ProcessControlPlanePod evaluates one Pod from the controller event stream.
func (m *Monitor) ProcessControlPlanePod(pod *corev1.Pod) error {
	if pod == nil || ComponentNameFromLabels(pod.Labels) == "" {
		return nil
	}
	if observation := DetectPodIssue(pod); observation != nil &&
		m.incidentSink != nil {
		m.incidentSink.Process(observation)
	}
	return nil
}

// SweepControlPlane evaluates the control-plane Pod cache after startup.
func (m *Monitor) SweepControlPlane() {
	m.mu.RLock()
	lister := m.podLister
	m.mu.RUnlock()
	if lister == nil {
		return
	}
	pods, err := lister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "controlplane sweep: failed to list pods from cache")
		return
	}
	for _, pod := range pods {
		if err := m.ProcessControlPlanePod(pod); err != nil {
			klog.ErrorS(err, "controlplane sweep: failed to process pod",
				"pod", klog.KObj(pod))
		}
	}
}

func (m *Monitor) nowTime() time.Time {
	return m.now()
}

func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()
	interval := time.Duration(m.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	m.check(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) ControlPlaneStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copyStatus := m.status
	copyStatus.Components = make(map[string]EndpointStatus, len(m.status.Components))
	for name, status := range m.status.Components {
		copyStatus.Components[name] = status
	}
	copyStatus.State = controlPlaneState(copyStatus)
	return copyStatus
}

// StatusJSON implements the health status boundary.
func (m *Monitor) StatusJSON() ([]byte, error) {
	return json.Marshal(m.ControlPlaneStatus())
}
