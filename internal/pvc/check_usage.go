package pvc

import (
	"context"
	"fmt"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type PvcUsage struct {
	Name            string
	PVName          string
	Namespace       string
	PodName         string
	UsagePercentage float64
}

// checkUsage iterates all nodes and queries the kubelet summary API for
// volume usage. Only PVCs that are actively mounted by a pod on the node
// appear in the summary. PVCs that are Bound but not yet mounted (e.g. a
// newly created PVC whose consumer pod hasn't scheduled) are invisible to
// this check and will not trigger alerts until a pod mounts them.
func (p *PvcMonitor) checkUsage(ctx context.Context) {
	nodes, err := k8s.GetNodes(ctx, p.client)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: failed to get nodes")
		return
	}

	nodeNames := make([]string, 0)
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	// Build PVC→PV name map once per cycle instead of N+1 Get calls
	pvByPVC := make(map[string]string)
	if pvcs, err := p.client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range pvcs.Items {
			c := &pvcs.Items[i]
			pvByPVC[c.Namespace+"/"+c.Name] = c.Spec.VolumeName
		}
	} else {
		klog.ErrorS(err, "pvc monitor: failed to list PVCs")
	}

	var pvcUsages []*PvcUsage
	incomplete := false

	for _, nodeName := range nodeNames {
		nodePvcUsage, err := p.getNodeUsage(ctx, nodeName, pvByPVC)
		if err != nil {
			klog.ErrorS(err, "pvc monitor: node usage failed", "node", nodeName)
			incomplete = true
			continue
		}
		pvcUsages = append(pvcUsages, nodePvcUsage...)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	currentNotified := make(map[string]bool, len(pvcUsages))

	clear := p.config.ClearThreshold
	if clear <= 0 || clear > p.config.Threshold {
		clear = p.config.Threshold // back-compat: 0 = no hysteresis
	}

	for _, pvc := range pvcUsages {
		if pvc.UsagePercentage >= p.config.Threshold {
			currentNotified[pvc.PVName] = true

			if p.firstScan {
				continue
			}

			severity := "normal"
			if pvc.UsagePercentage >= p.config.CriticalThreshold {
				severity = "high"
			}

			p.reportSignal(&event.Signal{
				Resource:  "pvc",
				PodName:   pvc.PodName,
				Namespace: pvc.Namespace,
				Reason:    "VolumeUsageHigh",
				Hint:      fmt.Sprintf("VolumeUsage(%.0f%%)", pvc.UsagePercentage),
				Severity:  severity,
				Owner:     pvc.PVName,
			})
		} else if p.notifiedPvc[pvc.PVName] && pvc.UsagePercentage >= clear {
			// HOLD: between clear and threshold — still firing, no re-signal
			currentNotified[pvc.PVName] = true
		}
	}

	if p.firstScan {
		p.firstScan = false
	}

	// Resolve previously notified PVCs that are now under threshold
	// Only when this cycle's data is complete (no per-node failures)
	if !incomplete {
		for pvName := range p.notifiedPvc {
			if !currentNotified[pvName] {
				p.correlator.ResolveByResource("pvc", pvName)
			}
		}
	}

	p.notifiedPvc = currentNotified
}
