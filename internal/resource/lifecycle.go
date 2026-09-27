package resource

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func NewMonitor(
	cfg Config,
	nodeLister corev1lister.NodeLister,
	podLister corev1lister.PodLister,
) *Monitor {
	return &Monitor{
		cfg: cfg, nodeLister: nodeLister, podLister: podLister,
		client: cfg.Client, interval: cfg.Interval,
	}
}

// Run starts the periodic check loop. The callback receives each signal.
func (m *Monitor) Run(
	ctx context.Context,
	callback func(obs *model.Observation),
	resolve func(node, reason string),
) {
	if m.interval <= 0 {
		m.interval = defaultCheckInterval
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			signals := m.Check()
			fsSignals, fsNodes := m.checkFilesystem(ctx)
			signals = append(signals, fsSignals...)
			for _, sig := range signals {
				callback(sig)
			}
			if resolve != nil {
				for _, r := range m.clearedReasons(signals, fsNodes) {
					resolve(r.node, r.reason)
				}
			}
		}
	}
}

type clearedReason struct{ node, reason string }

// reasonGroups are the mutually exclusive levels of one node signal. When a
// check no longer reports a level, the incident for it is resolved; without
// this, pressure incidents only closed as stale.
var (
	overcommitReasons = []string{
		constant.ReasonNodeResourceHigh, constant.ReasonNodeResourceCritical,
	}
	filesystemReasons = [][]string{
		{constant.ReasonNodeFilesystemHigh,
			constant.ReasonNodeFilesystemCritical},
		{constant.ReasonNodeInodesHigh, constant.ReasonNodeInodesCritical},
	}
)

// clearedReasons lists node reasons that this check evaluated and did not
// report. Filesystem reasons are only cleared for nodes whose summary was
// read, so an unreachable kubelet never resolves anything.
func (m *Monitor) clearedReasons(
	signals []*model.Observation, fsNodes []string,
) []clearedReason {
	active := make(map[string]bool, len(signals))
	for _, sig := range signals {
		active[sig.NodeName+"/"+sig.Reason] = true
	}
	nodes, err := m.nodeLister.List(labels.Everything())
	if err != nil {
		return nil
	}
	var out []clearedReason
	add := func(node string, reasons []string) {
		for _, reason := range reasons {
			if !active[node+"/"+reason] {
				out = append(out, clearedReason{node, reason})
			}
		}
	}
	for _, node := range nodes {
		add(node.Name, overcommitReasons)
	}
	for _, node := range fsNodes {
		for _, group := range filesystemReasons {
			add(node, group)
		}
	}
	return out
}
