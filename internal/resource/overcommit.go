package resource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// Check computes overcommit ratios for all nodes and returns signals.
func (m *Monitor) Check() []*model.Observation {
	nodes, err := m.nodeLister.List(labels.Everything())
	if err != nil {
		return nil
	}

	pods, err := m.podLister.List(labels.Everything())
	if err != nil {
		return nil
	}

	// Group pods by node, matching scheduler treatment of terminal pods.
	nodePods := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" ||
			pod.Status.Phase == corev1.PodSucceeded ||
			pod.Status.Phase == corev1.PodFailed {
			continue
		}
		nodePods[pod.Spec.NodeName] = append(
			nodePods[pod.Spec.NodeName], pod,
		)
	}

	var signals []*model.Observation
	for _, node := range nodes {
		sig := m.checkNode(node, nodePods[node.Name])
		if sig != nil {
			signals = append(signals, sig)
		}
	}
	return signals
}

func (m *Monitor) checkNode(
	node *corev1.Node, pods []*corev1.Pod,
) *model.Observation {
	cpuAlloc := node.Status.Allocatable.Cpu().MilliValue()
	memAlloc := node.Status.Allocatable.Memory().Value()

	if cpuAlloc <= 0 || memAlloc <= 0 {
		return nil
	}

	var cpuReq, memReq int64
	for _, pod := range pods {
		podCPU, podMemory := podRequests(pod)
		cpuReq += podCPU
		memReq += podMemory
	}

	cpuRatio := float64(cpuReq) / float64(cpuAlloc)
	memRatio := float64(memReq) / float64(memAlloc)
	reason, hint, severity := overcommitLevel(
		node.Name,
		cpuRatio,
		memRatio,
		m.cfg,
	)
	if reason == "" {
		return nil
	}
	return observe.Node(node, reason).
		WithSeverity(severity).WithHint(hint)
}

func overcommitLevel(
	nodeName string,
	cpuRatio, memRatio float64,
	cfg Config,
) (string, string, model.Severity) {
	critical := cpuRatio >= cfg.CpuCritical || memRatio >= cfg.MemCritical
	warning := cpuRatio >= cfg.CpuWarning || memRatio >= cfg.MemWarning
	if critical {
		hint := overcommitHint(nodeName, cpuRatio, memRatio, "critical")
		return constant.ReasonNodeResourceCritical, hint, model.SeverityCritical
	}
	if warning {
		hint := overcommitHint(nodeName, cpuRatio, memRatio, "high")
		return constant.ReasonNodeResourceHigh, hint, model.SeverityWarning
	}
	return "", "", ""
}

// podRequests returns scheduler-equivalent CPU and memory requests.
func podRequests(pod *corev1.Pod) (cpuMilli, memoryBytes int64) {
	for _, c := range pod.Spec.Containers {
		cpuMilli += c.Resources.Requests.Cpu().MilliValue()
		memoryBytes += c.Resources.Requests.Memory().Value()
	}
	var initCPU, initMemory int64
	for _, c := range pod.Spec.InitContainers {
		initCPU = max(initCPU, c.Resources.Requests.Cpu().MilliValue())
		initMemory = max(initMemory, c.Resources.Requests.Memory().Value())
	}
	cpuMilli = max(cpuMilli, initCPU)
	memoryBytes = max(memoryBytes, initMemory)
	if pod.Spec.Overhead != nil {
		cpuMilli += pod.Spec.Overhead.Cpu().MilliValue()
		memoryBytes += pod.Spec.Overhead.Memory().Value()
	}
	return cpuMilli, memoryBytes
}

func overcommitHint(
	nodeName string, cpuRatio, memRatio float64, level string,
) string {
	return fmt.Sprintf(
		"Node %s: %s overcommit — CPU %.1fx, Memory %.1fx "+
			"(based on pod requests vs allocatable)",
		nodeName, level, cpuRatio, memRatio,
	)
}
