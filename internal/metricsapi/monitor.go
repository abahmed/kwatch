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
	metrics    dynamic.NamespaceableResourceInterface
	client     kubernetes.Interface
	correlator *correlation.Engine
	cfg        config.RuntimeMetricsMonitor
	allowed    func(string) bool
	namespaces []string
	watchAll   bool
}

func New(restConfig *rest.Config, client kubernetes.Interface, cfg config.RuntimeMetricsMonitor, correlator *correlation.Engine) (*Monitor, error) {
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("metricsapi: create dynamic client: %w", err)
	}
	return &Monitor{
		metrics: dynamicClient.Resource(podMetricsGVR), client: client,
		correlator: correlator, cfg: cfg, watchAll: true,
	}, nil
}

func (m *Monitor) SetNamespaceFilter(filter func(string) bool) { m.allowed = filter }

func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
}

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
	namespaces := m.namespaces
	if m.watchAll {
		namespaces = []string{""}
	}
	for _, namespace := range namespaces {
		metricsList, err := m.metrics.Namespace(namespace).List(
			ctx, metav1.ListOptions{},
		)
		if err != nil {
			// Metrics Server is optional. A missing/forbidden metrics API must not
			// resolve existing incidents or create a synthetic outage incident.
			klog.V(2).InfoS("metricsapi unavailable", "error", err)
			continue
		}
		pods, err := m.listPods(ctx, namespace)
		if err != nil {
			klog.V(2).InfoS("metricsapi pod list unavailable", "error", err)
			continue
		}
		for i := range metricsList.Items {
			metrics := &metricsList.Items[i]
			pod := pods[metrics.GetNamespace()+"/"+metrics.GetName()]
			if pod != nil {
				m.processPod(pod, metrics)
			}
		}
	}
}

func (m *Monitor) listPods(
	ctx context.Context,
	namespace string,
) (map[string]*corev1.Pod, error) {
	const pageSize int64 = 500
	result := make(map[string]*corev1.Pod)
	continueToken := ""
	for {
		list, err := m.client.CoreV1().Pods(namespace).List(
			ctx,
			metav1.ListOptions{Limit: pageSize, Continue: continueToken},
		)
		if err != nil {
			return nil, err
		}
		for i := range list.Items {
			pod := &list.Items[i]
			if m.allowed == nil || m.allowed(pod.Namespace) {
				result[pod.Namespace+"/"+pod.Name] = pod
			}
		}
		if list.Continue == "" {
			return result, nil
		}
		continueToken = list.Continue
	}
}

func (m *Monitor) processPod(
	pod *corev1.Pod,
	metrics *unstructured.Unstructured,
) {
	namespace, name := metrics.GetNamespace(), metrics.GetName()
	if m.allowed != nil && !m.allowed(namespace) {
		return
	}
	if pod.Namespace != namespace || pod.Name != name {
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
		m.processMetric(pod, name, usage, limit, seen)
	}
	for name := range limits {
		if !seen[name] {
			m.resolveContainer(pod, name, constant.ReasonContainerMemoryHigh)
			m.resolveContainer(pod, name, constant.ReasonContainerCPUHigh)
		}
	}
}

func (m *Monitor) processMetric(pod *corev1.Pod, containerName string, usage map[string]string, limit corev1.ResourceList, seen map[string]bool) {
	seen[containerName] = true
	if usageMemory, ok := parseQuantity(usage[string(corev1.ResourceMemory)]); ok {
		if sig := usageSignal(pod, containerName, usageMemory, limit.Memory(), m.cfg.MemoryWarningPercent, m.cfg.MemoryCriticalPercent, constant.ReasonContainerMemoryHigh, "memory"); sig != nil {
			m.report(sig)
		} else {
			m.resolveContainer(pod, containerName, constant.ReasonContainerMemoryHigh)
		}
	}
	if usageCPU, ok := parseQuantity(usage[string(corev1.ResourceCPU)]); ok {
		if sig := usageSignal(pod, containerName, usageCPU, limit.Cpu(), m.cfg.CPUWarningPercent, m.cfg.CPUCriticalPercent, constant.ReasonContainerCPUHigh, "cpu"); sig != nil {
			m.report(sig)
		} else {
			m.resolveContainer(pod, containerName, constant.ReasonContainerCPUHigh)
		}
	}
}

func usageSignal(pod *corev1.Pod, container string, usage, limit *resource.Quantity, warning, critical int, reason, unit string) *event.Signal {
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
	owner := pod.Namespace + "/" + pod.Name
	if len(pod.OwnerReferences) == 0 {
		owner = pod.Name
	}
	return &event.Signal{Resource: "pod", Namespace: pod.Namespace, PodName: pod.Name, PodUID: string(pod.UID), PodLineageID: pod.Annotations[event.PodLineageAnnotation], Container: container,
		Owner: owner, Reason: reason, Labels: pod.Labels, Severity: severity,
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
	m.correlator.Process(event.Event{Resource: sig.Resource, Namespace: sig.Namespace, PodName: sig.PodName, PodUID: sig.PodUID, PodLineageID: sig.PodLineageID, ContainerName: sig.Container, Reason: sig.Reason, Hint: sig.Hint, Labels: sig.Labels, Severity: sig.Severity}, sig.Owner, nil)
}

func (m *Monitor) resolveContainer(pod *corev1.Pod, container, reason string) {
	owner := pod.Namespace + "/" + pod.Name
	if len(pod.OwnerReferences) == 0 {
		owner = pod.Name
	}
	m.correlator.MarkResolved(correlation.IncidentKey(event.Event{Resource: "pod", Namespace: pod.Namespace, PodName: pod.Name, PodUID: string(pod.UID), PodLineageID: pod.Annotations[event.PodLineageAnnotation], ContainerName: container, Reason: reason}, owner, nil))
}
