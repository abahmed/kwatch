package pvc

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type PvcUsage struct {
	Name            string
	PVName          string
	Namespace       string
	PodName         string
	UsagePercentage float64
}

func (p *PvcMonitor) checkUsage() {
	nodes, err := k8s.GetNodes(p.client)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: failed to get nodes")
		return
	}

	nodeNames := make([]string, 0)
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	var pvcUsages []*PvcUsage

	for _, nodeName := range nodeNames {
		nodePvcUsage, _ := p.getNodeUsage(nodeName)
		pvcUsages = append(pvcUsages, nodePvcUsage...)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	currentNotified := make(map[string]bool, len(pvcUsages))

	for _, pvc := range pvcUsages {
		if pvc.UsagePercentage >= p.config.Threshold {
			currentNotified[pvc.PVName] = true

			severity := "normal"
			if pvc.UsagePercentage >= p.config.CriticalThreshold {
				severity = "high"
			}

			ev := event.Event{
				Resource:  "pvc",
				PodName:   pvc.PodName,
				Namespace: pvc.Namespace,
				NodeName:  "",
				Reason:    "VolumeUsageHigh",
				Logs:      "",
				Labels:    nil,
				Hint:      fmt.Sprintf("VolumeUsage(%.0f%%)", pvc.UsagePercentage),
				Severity:  severity,
			}

			inc, action := p.correlator.Process(ev, pvc.PVName, nil)
			if action != model.ActionSkip {
				p.alertManager.NotifyIncident(inc, action)
			}
		}
	}

	// Resolve previously notified PVCs that are now under threshold
	for pvName := range p.notifiedPvc {
		if !currentNotified[pvName] {
			p.correlator.ResolveByResource("pvc", pvName)
		}
	}

	p.notifiedPvc = currentNotified
}
