package controlplane

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/metrics"
)

const (
	defaultInterval        = 30 * time.Second
	defaultProbeTimeout    = 10 * time.Second
	defaultFailureSamples  = 2
	defaultRecoverySamples = 2
)

type EndpointStatus struct {
	Name        string        `json:"name"`
	Available   bool          `json:"available"`
	Latency     time.Duration `json:"latency"`
	LastError   string        `json:"lastError,omitempty"`
	LastChecked time.Time     `json:"lastChecked"`
	Supported   bool          `json:"supported"`
}

type Status struct {
	State       string                    `json:"state"`
	LastCheck   time.Time                 `json:"lastCheck"`
	APIServer   EndpointStatus            `json:"apiServer"`
	CoreDNS     EndpointStatus            `json:"coreDNS"`
	Components  map[string]EndpointStatus `json:"components"`
	ProbeErrors int64                     `json:"probeErrors"`
}

type Monitor struct {
	client     kubernetes.Interface
	restClient rest.Interface
	cfg        config.ControlPlaneMonitor
	correlator *correlation.Engine
	mu         sync.RWMutex
	status     Status
	failures   map[string]int
	recoveries map[string]int
	now        func() time.Time
	plan       feature.Plan
}

func New(restConfig *rest.Config, client kubernetes.Interface, cfg config.ControlPlaneMonitor, correlator *correlation.Engine) (*Monitor, error) {
	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("controlplane: create REST client: %w", err)
	}
	return &Monitor{client: client, restClient: restClient, cfg: cfg, correlator: correlator,
		status: Status{Components: make(map[string]EndpointStatus)}, failures: make(map[string]int), recoveries: make(map[string]int), now: time.Now}, nil
}

// SetClock injects the wall clock used for status timestamps and probe latency.
func (m *Monitor) SetClock(now func() time.Time) {
	if now != nil {
		m.now = now
	}
}

func (m *Monitor) nowTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return clock.Now()
}

// SetFeaturePlan applies signal-level decisions while keeping the monitor
// independent from licensing and configuration parsing.
func (m *Monitor) SetFeaturePlan(plan feature.Plan) { m.plan = plan }

func (m *Monitor) Start(ctx context.Context) {
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
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) ControlPlaneStatus() interface{} {
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

func controlPlaneState(status Status) string {
	if !status.APIServer.Supported && status.CoreDNS.Supported == false {
		return "unavailable"
	}
	if !status.APIServer.Available || (status.CoreDNS.Supported && !status.CoreDNS.Available) {
		return "partial"
	}
	for _, component := range status.Components {
		if component.Supported && !component.Available {
			return "partial"
		}
	}
	if status.LastCheck.IsZero() {
		return "unavailable"
	}
	return "healthy"
}

func (m *Monitor) check(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
	defer cancel()
	// Record the attempt before any individual probe can fail. Otherwise a
	// failed pod discovery leaves LastCheck and component statuses from an
	// older successful sweep, which can make the health endpoint look healthy.
	m.mu.Lock()
	m.status.LastCheck = m.nowTime()
	m.mu.Unlock()
	if m.featureEnabled(feature.APIServerHealth) {
		m.checkAPIServer(probeCtx)
	}
	if m.featureEnabled(feature.NetworkDetection) {
		m.checkCoreDNS(probeCtx)
	}
	components := []string{}
	if m.featureEnabled(feature.SchedulerHealth) {
		components = append(components, "kube-scheduler")
	}
	if m.featureEnabled(feature.ControllerManagerHealth) {
		components = append(components, "kube-controller-manager")
	}
	if m.featureEnabled(feature.EtcdHealth) {
		components = append(components, "etcd")
	}
	if len(components) == 0 {
		return
	}
	pods, err := m.client.CoreV1().Pods("").List(probeCtx, metav1.ListOptions{})
	if err != nil {
		m.markComponentsUnavailable(err)
		m.recordProbeError("control-plane pod discovery", err)
		return
	}
	for _, component := range components {
		m.checkComponent(probeCtx, component, pods.Items)
	}
	m.mu.Lock()
	m.status.LastCheck = m.nowTime()
	m.mu.Unlock()
}

func (m *Monitor) markComponentsUnavailable(err error) {
	checked := m.nowTime()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Components == nil {
		m.status.Components = make(map[string]EndpointStatus)
	}
	components := []string{}
	if m.featureEnabled(feature.SchedulerHealth) {
		components = append(components, "kube-scheduler")
	}
	if m.featureEnabled(feature.ControllerManagerHealth) {
		components = append(components, "kube-controller-manager")
	}
	if m.featureEnabled(feature.EtcdHealth) {
		components = append(components, "etcd")
	}
	for _, component := range components {
		m.status.Components[component] = EndpointStatus{
			Name:        component,
			Available:   false,
			LastError:   err.Error(),
			LastChecked: checked,
			Supported:   true,
		}
	}
}

func (m *Monitor) checkCoreDNS(ctx context.Context) {
	started := m.nowTime()
	_, err := net.DefaultResolver.LookupHost(ctx, "kubernetes.default.svc")
	checked := m.nowTime()
	status := EndpointStatus{Name: "coredns/kubernetes.default.svc", Latency: checked.Sub(started), LastChecked: checked, Supported: true, Available: err == nil}
	if err != nil {
		status.LastError = err.Error()
	}
	m.mu.Lock()
	m.status.CoreDNS = status
	m.mu.Unlock()
	if err != nil {
		m.observe("coredns", false, constant.ReasonCoreDNSUnavailable, fmt.Sprintf("DNS lookup kubernetes.default.svc failed: %v", err))
		return
	}
	m.observe("coredns", true, constant.ReasonCoreDNSUnavailable, "CoreDNS lookup recovered")
}

func (m *Monitor) checkAPIServer(ctx context.Context) {
	started := m.nowTime()
	_, err := m.restClient.Get().AbsPath("/readyz").Param("verbose", "true").Do(ctx).Raw()
	checked := m.nowTime()
	status := EndpointStatus{Name: "kube-apiserver/readyz", Latency: checked.Sub(started), LastChecked: checked, Supported: true, Available: err == nil}
	if err != nil {
		status.LastError = err.Error()
	}
	m.mu.Lock()
	m.status.APIServer = status
	m.mu.Unlock()
	metrics.DefaultRegistry().APIServerLatencyMs.Store(
		status.Latency.Milliseconds(),
	)
	if err != nil {
		metrics.DefaultRegistry().APIServerProbeErrors.Add(1)
		m.observe("api-server", false, constant.ReasonAPIServerUnavailable, fmt.Sprintf("kube-apiserver /readyz failed: %v", err))
		return
	}
	threshold := time.Duration(m.cfg.APIServerLatencyWarningMs) * time.Millisecond
	if threshold <= 0 {
		threshold = time.Second
	}
	if m.featureEnabled(feature.APIServerLatency) && status.Latency >= threshold {
		m.observe("api-server-latency", false, constant.ReasonAPIServerLatency, fmt.Sprintf("kube-apiserver /readyz took %s (threshold %s)", status.Latency.Round(time.Millisecond), threshold))
		return
	}
	m.observe("api-server", true, constant.ReasonAPIServerUnavailable, "kube-apiserver /readyz recovered")
	m.observe("api-server-latency", true, constant.ReasonAPIServerLatency, "kube-apiserver /readyz latency recovered")
}

func (m *Monitor) checkComponent(ctx context.Context, component string, pods []corev1.Pod) {
	var candidates []corev1.Pod
	for i := range pods {
		if isComponentPod(&pods[i], component) {
			candidates = append(candidates, pods[i])
		}
	}
	if len(candidates) == 0 {
		m.setComponent(component, EndpointStatus{Name: component, Supported: false, LastChecked: m.nowTime()})
		return
	}
	var lastErr error
	var latency time.Duration
	for _, pod := range candidates {
		started := m.nowTime()
		path := "healthz"
		if component == "etcd" {
			path = "health"
		}
		_, err := m.client.CoreV1().RESTClient().Get().Namespace(pod.Namespace).Resource("pods").Name(pod.Name).SubResource("proxy").Suffix(path).Do(ctx).Raw()
		checked := m.nowTime()
		latency = checked.Sub(started)
		if err == nil {
			m.setComponent(component, EndpointStatus{Name: component, Supported: true, Available: true, Latency: latency, LastChecked: checked})
			m.observe(component, true, componentReason(component), component+" health endpoint recovered")
			return
		}
		lastErr = err
	}
	status := EndpointStatus{Name: component, Supported: true, Available: false, Latency: latency, LastChecked: m.nowTime()}
	if lastErr != nil {
		status.LastError = lastErr.Error()
		metrics.DefaultRegistry().ControlPlaneProbeErrors.Add(1)
	}
	m.setComponent(component, status)
	m.observe(component, false, componentReason(component), fmt.Sprintf("%s health endpoint failed: %v", component, lastErr))
}

func isComponentPod(pod *corev1.Pod, component string) bool {
	if pod == nil || pod.Status.Phase == corev1.PodSucceeded {
		return false
	}
	return pod.Labels["component"] == component || pod.Labels["k8s-app"] == component
}

func componentReason(component string) string {
	switch component {
	case "kube-scheduler":
		return constant.ReasonSchedulerUnavailable
	case "kube-controller-manager":
		return constant.ReasonControllerManagerUnavailable
	case "etcd":
		return constant.ReasonEtcdUnavailable
	}
	return constant.ReasonControlPlaneComponentFailure
}

func (m *Monitor) setComponent(name string, status EndpointStatus) {
	m.mu.Lock()
	m.status.Components[name] = status
	m.mu.Unlock()
}

func (m *Monitor) recordProbeError(name string, err error) {
	m.mu.Lock()
	m.status.ProbeErrors++
	m.mu.Unlock()
	metrics.DefaultRegistry().ControlPlaneProbeErrors.Add(1)
	klog.ErrorS(err, "controlplane probe failed", "probe", name)
}

func (m *Monitor) observe(key string, healthy bool, reason, hint string) {
	threshold := m.cfg.FailureThreshold
	if threshold <= 0 {
		threshold = defaultFailureSamples
	}
	recovery := m.cfg.RecoveryThreshold
	if recovery <= 0 {
		recovery = defaultRecoverySamples
	}
	m.mu.Lock()
	if healthy {
		m.recoveries[key]++
		m.failures[key] = 0
	} else {
		m.failures[key]++
		m.recoveries[key] = 0
	}
	failed := m.failures[key] >= threshold
	resolved := healthy && m.recoveries[key] >= recovery
	m.mu.Unlock()
	if !healthy && failed {
		m.correlator.Process(event.Event{Resource: "controlplane", Reason: reason, Hint: hint, Severity: "high"}, key, nil)
	}
	if resolved {
		m.correlator.MarkResolved(correlation.BuildKey("", key, reason, ""))
	}
}

func (m *Monitor) featureEnabled(id feature.ID) bool {
	if len(m.plan.Decisions) == 0 {
		return true
	}
	return m.plan.Enabled(id)
}
