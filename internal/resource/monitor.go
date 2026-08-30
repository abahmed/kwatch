package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
)

type Config struct {
	CpuWarning                float64
	CpuCritical               float64
	MemWarning                float64
	MemCritical               float64
	FilesystemWarningPercent  float64
	FilesystemCriticalPercent float64
	InodeWarningPercent       float64
	InodeCriticalPercent      float64
	Interval                  time.Duration
	Client                    kubernetes.Interface
}

type Monitor struct {
	cfg        Config
	nodeLister corev1lister.NodeLister
	podLister  corev1lister.PodLister
	client     kubernetes.Interface
	interval   time.Duration
}

func NewMonitor(cfg Config, nodeLister corev1lister.NodeLister, podLister corev1lister.PodLister) *Monitor {
	return &Monitor{cfg: cfg, nodeLister: nodeLister, podLister: podLister, client: cfg.Client, interval: cfg.Interval}
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
			signals = append(signals, m.checkFilesystem(ctx)...)
			for _, sig := range signals {
				callback(sig)
			}
		}
	}
}

type filesystemSummary struct {
	Node struct {
		FS *filesystemStats `json:"fs"`
	} `json:"node"`
}

type filesystemStats struct {
	CapacityBytes *uint64 `json:"capacityBytes"`
	UsedBytes     *uint64 `json:"usedBytes"`
	Inodes        *uint64 `json:"inodes"`
	InodesFree    *uint64 `json:"inodesFree"`
}

func (m *Monitor) checkFilesystem(ctx context.Context) []*event.Signal {
	if m.client == nil || (m.cfg.FilesystemWarningPercent <= 0 && m.cfg.InodeWarningPercent <= 0) {
		return nil
	}
	nodes, err := m.nodeLister.List(labels.Everything())
	if err != nil {
		return nil
	}
	var signals []*event.Signal
	for _, node := range nodes {
		body, err := k8s.GetNodeSummary(ctx, m.client, node.Name)
		if err != nil {
			continue
		}
		var summary filesystemSummary
		if json.Unmarshal(body, &summary) != nil || summary.Node.FS == nil {
			continue
		}
		signals = append(signals, filesystemSignals(node, summary.Node.FS, m.cfg)...)
	}
	return signals
}

func filesystemSignals(node *corev1.Node, fs *filesystemStats, cfg Config) []*event.Signal {
	var out []*event.Signal
	if fs.CapacityBytes != nil && fs.UsedBytes != nil && *fs.CapacityBytes > 0 {
		pct := float64(*fs.UsedBytes) / float64(*fs.CapacityBytes) * 100
		if sig := thresholdSignal(node, pct, cfg.FilesystemWarningPercent, cfg.FilesystemCriticalPercent, constant.ReasonNodeFilesystemHigh, constant.ReasonNodeFilesystemCritical, "filesystem"); sig != nil {
			out = append(out, sig)
		}
	}
	if fs.Inodes != nil && fs.InodesFree != nil && *fs.Inodes > 0 && *fs.InodesFree <= *fs.Inodes {
		pct := float64(*fs.Inodes-*fs.InodesFree) / float64(*fs.Inodes) * 100
		if sig := thresholdSignal(node, pct, cfg.InodeWarningPercent, cfg.InodeCriticalPercent, constant.ReasonNodeInodesHigh, constant.ReasonNodeInodesCritical, "inodes"); sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

func thresholdSignal(node *corev1.Node, pct, warning, critical float64, warnReason, criticalReason, resourceName string) *event.Signal {
	if warning <= 0 || pct < warning {
		return nil
	}
	reason, severity := warnReason, model.SeverityWarning
	if critical > 0 && pct >= critical {
		reason, severity = criticalReason, model.SeverityCritical
	}
	return &event.Signal{Resource: "node", NodeName: node.Name, PodName: node.Name, Owner: node.Name, Reason: reason, Severity: severity,
		Labels: node.Labels, Hint: fmt.Sprintf("Node %s %s usage is %.1f%%", node.Name, resourceName, pct)}
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
		podCPU, podMemory := podRequests(pod)
		cpuReq += podCPU
		memReq += podMemory
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

// podRequests returns the scheduler-equivalent CPU and memory requests for a
// pod. Regular containers are summed, while init-container requests are the
// per-resource maximum. Pod overhead is charged on top of that effective
// request. Ignoring either part understates node overcommitment.
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

func overcommitHint(nodeName string, cpuRatio, memRatio float64, level string) string {
	return fmt.Sprintf("Node %s: %s overcommit — CPU %.1fx, Memory %.1fx (based on pod requests vs allocatable)",
		nodeName, level, cpuRatio, memRatio)
}
