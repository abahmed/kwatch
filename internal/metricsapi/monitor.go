package metricsapi

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

var podMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}

type Monitor struct {
	metrics    dynamic.ResourceInterface
	client     kubernetes.Interface
	correlator *correlation.Engine
	cfg        config.RuntimeMetricsMonitor
	allowed    func(string) bool
}

func New(restConfig *rest.Config, client kubernetes.Interface, cfg config.RuntimeMetricsMonitor, correlator *correlation.Engine) (*Monitor, error) {
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("metricsapi: create dynamic client: %w", err)
	}
	return &Monitor{metrics: dynamicClient.Resource(podMetricsGVR), client: client, correlator: correlator, cfg: cfg}, nil
}

func (m *Monitor) SetNamespaceFilter(filter func(string) bool) { m.allowed = filter }

func (m *Monitor) Start(ctx context.Context) {
	interval := time.Duration(m.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	m.sweep(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweep(ctx)
		}
	}
}

func (m *Monitor) sweep(ctx context.Context) {
	if m.metrics == nil || m.client == nil {
		return
	}
	metricsList, err := m.metrics.List(ctx, metav1.ListOptions{})
	if err != nil {
		// Metrics Server is optional. A missing/forbidden metrics API must not
		// resolve existing incidents or create a synthetic outage incident.
		klog.V(2).InfoS("metricsapi unavailable", "error", err)
		return
	}
	for i := range metricsList.Items {
		m.processPod(ctx, &metricsList.Items[i])
	}
}

func (m *Monitor) processPod(ctx context.Context, metrics *unstructured.Unstructured) {
	namespace, name := metrics.GetNamespace(), metrics.GetName()
	if m.allowed != nil && !m.allowed(namespace) {
		return
	}
	pod, err := m.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return
	}
	limits := containerLimits(pod)
	containers, found, _ := unstructured.NestedSlice(metrics.Object, "containers")
	if !found {
		return
	}
	seen := make(map[string]bool)
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := container["name"].(string)
		limit, ok := limits[name]
		if !ok {
			continue
		}
		usage, _, _ := unstructured.NestedStringMap(container, "usage")
		m.processMetric(namespace, metrics.GetName(), name, usage, limit, seen, pod.Labels)
	}
	for name := range limits {
		if !seen[name] {
			m.resolveContainer(namespace, metrics.GetName(), name, constant.ReasonContainerMemoryHigh)
			m.resolveContainer(namespace, metrics.GetName(), name, constant.ReasonContainerCPUHigh)
		}
	}
}

func (m *Monitor) processMetric(namespace, podName, containerName string, usage map[string]string, limit corev1.ResourceList, seen map[string]bool, labels map[string]string) {
	seen[containerName] = true
	if usageMemory, ok := parseQuantity(usage[string(corev1.ResourceMemory)]); ok {
		if sig := usageSignal(namespace, podName, containerName, usageMemory, limit.Memory(), m.cfg.MemoryWarningPercent, m.cfg.MemoryCriticalPercent, constant.ReasonContainerMemoryHigh, labels, "memory"); sig != nil {
			m.report(sig)
		} else {
			m.resolveContainer(namespace, podName, containerName, constant.ReasonContainerMemoryHigh)
		}
	}
	if usageCPU, ok := parseQuantity(usage[string(corev1.ResourceCPU)]); ok {
		if sig := usageSignal(namespace, podName, containerName, usageCPU, limit.Cpu(), m.cfg.CPUWarningPercent, m.cfg.CPUCriticalPercent, constant.ReasonContainerCPUHigh, labels, "cpu"); sig != nil {
			m.report(sig)
		} else {
			m.resolveContainer(namespace, podName, containerName, constant.ReasonContainerCPUHigh)
		}
	}
}

func usageSignal(namespace, pod, container string, usage, limit *resource.Quantity, warning, critical int, reason string, labels map[string]string, unit string) *event.Signal {
	if limit == nil || limit.IsZero() || usage == nil {
		return nil
	}
	percent := usage.AsApproximateFloat64() / limit.AsApproximateFloat64() * 100
	if percent < float64(warning) {
		return nil
	}
	severity := model.SeverityWarning
	if percent >= float64(critical) {
		severity = model.SeverityCritical
	}
	return &event.Signal{Resource: "pod", Namespace: namespace, PodName: pod, Container: container,
		Owner: namespace + "/" + pod, Reason: reason, Labels: labels, Severity: severity,
		Hint: fmt.Sprintf("container %s %s usage is %.0f%% of its limit (%s/%s)", container, unit, percent, usage.String(), limit.String())}
}

func containerLimits(pod *corev1.Pod) map[string]corev1.ResourceList {
	limits := make(map[string]corev1.ResourceList)
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		limits[container.Name] = container.Resources.Limits
	}
	return limits
}

func parseQuantity(value string) (*resource.Quantity, bool) {
	if value == "" {
		return nil, false
	}
	quantity, err := resource.ParseQuantity(value)
	return &quantity, err == nil
}

func (m *Monitor) report(sig *event.Signal) {
	m.correlator.Process(event.Event{Resource: sig.Resource, Namespace: sig.Namespace, PodName: sig.PodName, ContainerName: sig.Container, Reason: sig.Reason, Hint: sig.Hint, Labels: sig.Labels, Severity: sig.Severity}, sig.Owner, nil)
}

func (m *Monitor) resolveContainer(namespace, pod, container, reason string) {
	m.correlator.MarkResolved(correlation.BuildKey(namespace, namespace+"/"+pod, reason, container))
}
