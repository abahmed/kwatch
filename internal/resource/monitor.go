package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type Config struct {
	CpuWarning  float64
	CpuCritical float64
	MemWarning  float64
	MemCritical float64
	Interval    time.Duration
}

type Monitor struct {
	cfg        Config
	nodeLister corev1lister.NodeLister
	podLister  corev1lister.PodLister
	interval   time.Duration
}

func NewMonitor(cfg Config, nodeLister corev1lister.NodeLister, podLister corev1lister.PodLister) *Monitor {
	return &Monitor{cfg: cfg, nodeLister: nodeLister, podLister: podLister, interval: cfg.Interval}
}

// Run starts the periodic check loop. The callback is called for each
// overcommit signal found.
func (m *Monitor) Run(ctx context.Context, callback func(sig *event.Signal)) {
	if m.interval <= 0 {
		m.interval = 300 * time.Second
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.interval):
			signals := m.Check()
			for _, sig := range signals {
				callback(sig)
			}
		}
	}
}

// Check computes overcommit ratios for all nodes and returns signals.
func (m *Monitor) Check() []*event.Signal {
	nodes, err := m.nodeLister.List(labels.Everything())
	if err != nil {
		return nil
	}

	pods, err := m.podLister.List(labels.Everything())
	if err != nil {
		return nil
	}

	// Group pods by node
	nodePods := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pod)
	}

	var signals []*event.Signal
	for _, node := range nodes {
		sig := m.checkNode(node, nodePods[node.Name])
		if sig != nil {
			signals = append(signals, sig)
		}
	}
	return signals
}

func (m *Monitor) checkNode(node *corev1.Node, pods []*corev1.Pod) *event.Signal {
	cpuAlloc := node.Status.Allocatable.Cpu().MilliValue()
	memAlloc := node.Status.Allocatable.Memory().Value()

	if cpuAlloc <= 0 || memAlloc <= 0 {
		return nil
	}

	var cpuReq, memReq int64
	for _, pod := range pods {
		for _, c := range pod.Spec.Containers {
			cpuReq += c.Resources.Requests.Cpu().MilliValue()
			memReq += c.Resources.Requests.Memory().Value()
		}
	}

	cpuRatio := float64(cpuReq) / float64(cpuAlloc)
	memRatio := float64(memReq) / float64(memAlloc)

	var reason, hint string
	var severity model.Severity

	switch {
	case cpuRatio >= m.cfg.CpuCritical && memRatio >= m.cfg.MemCritical:
		reason = constant.ReasonNodeResourceCritical
		hint = overcommitHint(node.Name, cpuRatio, memRatio, "critical")
		severity = model.SeverityCritical
	case cpuRatio >= m.cfg.CpuCritical:
		reason = constant.ReasonNodeResourceCritical
		hint = overcommitHint(node.Name, cpuRatio, memRatio, "critical")
		severity = model.SeverityCritical
	case memRatio >= m.cfg.MemCritical:
		reason = constant.ReasonNodeResourceCritical
		hint = overcommitHint(node.Name, cpuRatio, memRatio, "critical")
		severity = model.SeverityCritical
	case cpuRatio >= m.cfg.CpuWarning && memRatio >= m.cfg.MemWarning:
		reason = constant.ReasonNodeResourceHigh
		hint = overcommitHint(node.Name, cpuRatio, memRatio, "high")
		severity = model.SeverityWarning
	case cpuRatio >= m.cfg.CpuWarning:
		reason = constant.ReasonNodeResourceHigh
		hint = overcommitHint(node.Name, cpuRatio, memRatio, "high")
		severity = model.SeverityWarning
	case memRatio >= m.cfg.MemWarning:
		reason = constant.ReasonNodeResourceHigh
		hint = overcommitHint(node.Name, cpuRatio, memRatio, "high")
		severity = model.SeverityWarning
	default:
		return nil
	}

	return &event.Signal{
		Resource: "node",
		Reason:   reason,
		Hint:     hint,
		NodeName: node.Name,
		Owner:    node.Name,
		Labels:   node.Labels,
		Severity: severity,
	}
}

func overcommitHint(nodeName string, cpuRatio, memRatio float64, level string) string {
	return fmt.Sprintf("Node %s: %s overcommit — CPU %.1fx, Memory %.1fx (based on pod requests vs allocatable)",
		nodeName, level, cpuRatio, memRatio)
}
