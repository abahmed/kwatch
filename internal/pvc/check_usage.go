package pvc

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/k8s"
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

	for _, pvc := range pvcUsages {
		if pvc.UsagePercentage >= p.config.Threshold {
			if _, ok := p.notifiedPvc[pvc.PVName]; ok {
				continue
			}

			msg := fmt.Sprintf("Volume Usage for %s (%s) attached to pod %s "+
				"in namespace %s is %.2f%% (higher than %.0f%%)",
				pvc.Name,
				pvc.PVName,
				pvc.PodName,
				pvc.Namespace,
				pvc.UsagePercentage,
				p.config.Threshold,
			)
			p.alertManager.Notify(msg)
			p.notifiedPvc[pvc.PVName] = true
		}
	}
}
