package resource

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) checkFilesystem(
	ctx context.Context,
) []*model.Observation {
	if m.client == nil ||
		(m.cfg.FilesystemWarningPercent <= 0 &&
			m.cfg.InodeWarningPercent <= 0) {
		return nil
	}
	nodes, err := m.nodeLister.List(labels.Everything())
	if err != nil {
		return nil
	}
	var signals []*model.Observation
	for _, node := range nodes {
		body, err := k8s.GetNodeSummary(ctx, m.client, node.Name)
		if err != nil {
			continue
		}
		var summary filesystemSummary
		if json.Unmarshal(body, &summary) != nil || summary.Node.FS == nil {
			continue
		}
		signals = append(
			signals,
			filesystemSignals(node, summary.Node.FS, m.cfg)...,
		)
	}
	return signals
}

func filesystemSignals(
	node *corev1.Node, fs *filesystemStats, cfg Config,
) []*model.Observation {
	var out []*model.Observation
	if fs.CapacityBytes != nil && fs.UsedBytes != nil && *fs.CapacityBytes > 0 {
		pct := float64(*fs.UsedBytes) / float64(*fs.CapacityBytes) * 100
		if sig := thresholdSignal(
			node, pct, cfg.FilesystemWarningPercent,
			cfg.FilesystemCriticalPercent,
			constant.ReasonNodeFilesystemHigh,
			constant.ReasonNodeFilesystemCritical,
			"filesystem",
		); sig != nil {
			out = append(out, sig)
		}
	}
	if fs.Inodes != nil && fs.InodesFree != nil &&
		*fs.Inodes > 0 && *fs.InodesFree <= *fs.Inodes {
		pct := float64(*fs.Inodes-*fs.InodesFree) / float64(*fs.Inodes) * 100
		if sig := thresholdSignal(
			node, pct, cfg.InodeWarningPercent,
			cfg.InodeCriticalPercent,
			constant.ReasonNodeInodesHigh,
			constant.ReasonNodeInodesCritical,
			"inodes",
		); sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

func thresholdSignal(
	node *corev1.Node,
	pct, warning, critical float64,
	warnReason, criticalReason, resourceName string,
) *model.Observation {
	if warning <= 0 || pct < warning {
		return nil
	}
	reason, severity := warnReason, model.SeverityWarning
	if critical > 0 && pct >= critical {
		reason, severity = criticalReason, model.SeverityCritical
	}
	return observe.Node(node, reason).WithSeverity(severity).
		WithHint(fmt.Sprintf(
			"Node %s %s usage is %.1f%%", node.Name, resourceName, pct,
		))
}
