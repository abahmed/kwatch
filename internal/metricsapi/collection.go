package metricsapi

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
)

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
	if m.podLister != nil {
		if result, err := m.podsFromCache(namespace); err == nil {
			return result, nil
		}
	}
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
			m.resolveContainer(
				pod, name, constant.ReasonContainerMemoryHigh,
			)
			m.resolveContainer(
				pod, name, constant.ReasonContainerCPUHigh,
			)
		}
	}
}

func (m *Monitor) podsFromCache(
	namespace string,
) (map[string]*corev1.Pod, error) {
	var (
		pods []*corev1.Pod
		err  error
	)
	if namespace == "" {
		pods, err = m.podLister.List(labels.Everything())
	} else {
		pods, err = m.podLister.Pods(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]*corev1.Pod, len(pods))
	for _, pod := range pods {
		if m.allowed == nil || m.allowed(pod.Namespace) {
			result[pod.Namespace+"/"+pod.Name] = pod
		}
	}
	return result, nil
}
