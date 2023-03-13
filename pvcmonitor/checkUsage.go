package pvcmonitor

import (
	"fmt"

	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
)

type PvcUsage struct {
	Name            string
	PVName          string
	Namespace       string
	PodName         string
	UsagePercentage float64
}

func (p *PvcMonitor) checkUsage() {
	// getting nodes
	nodes, err := util.GetNodes(p.client)
	if err != nil {
		logrus.Errorf("pvc monitor: failed to get nodes %s", err.Error())
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

	for _, pvc := range pvcUsages {
		if pvc.UsagePercentage >= p.config.Threshold {
			// ignore notified pv
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
