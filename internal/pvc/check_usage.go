package pvc

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/state"
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

	p.apply(pvcUsages, pvByPVC, incomplete)
}

// SampleNode reads stats/summary for ONE node out-of-cycle and folds the result
// into the cache + correlator. Called from the pod informer when a tracked PVC's
// pod goes Running. Debounces to ≤1 kubelet read per node per window.
func (p *PvcMonitor) SampleNode(ctx context.Context, nodeName string) {
	if !p.config.Enabled || nodeName == "" {
		return
	}
	now := time.Now()
	p.mu.Lock()
	if p.lastNodeSample == nil {
		p.lastNodeSample = make(map[string]time.Time)
	}
	if last, ok := p.lastNodeSample[nodeName]; ok && now.Sub(last) < nodeSampleDebounce {
		p.mu.Unlock()
		return
	}
	p.lastNodeSample[nodeName] = now
	p.mu.Unlock()

	pvByPVC := make(map[string]string)
	if pvcs, err := p.client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range pvcs.Items {
			c := &pvcs.Items[i]
			pvByPVC[c.Namespace+"/"+c.Name] = c.Spec.VolumeName
		}
	} else {
		klog.ErrorS(err, "pvc monitor: SampleNode failed to list PVCs")
		return
	}

	usages, err := p.getNodeUsage(ctx, nodeName, pvByPVC)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: SampleNode usage failed", "node", nodeName)
		return
	}

	// incomplete=true: single-node view is partial, so update+signal but DON'T
	// run cluster-wide resolves (those stay with the periodic full sweep).
	p.apply(usages, pvByPVC, true)
}

// apply folds one batch of observations into the cache + correlator under p.mu.
// Pure in-memory — no K8s writes. incomplete=true means "partial view" (single
// node / per-node error): update+signal but skip the cluster-wide unmounted/deleted
// resolve pass (only the full sweep owns resolves).
func (p *PvcMonitor) apply(pvcUsages []*PvcUsage, pvByPVC map[string]string, incomplete bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	currentNotified := make(map[string]bool, len(pvcUsages))
	seen := make(map[string]bool, len(pvcUsages))

	clear := p.config.ClearThreshold
	if clear <= 0 || clear > p.config.Threshold {
		clear = p.config.Threshold
	}

	for _, pvc := range pvcUsages {
		seen[pvc.PVName] = true
		p.lastUsage[pvc.PVName] = state.PvcSample{
			Pct: pvc.UsagePercentage, Namespace: pvc.Namespace,
			Name: pvc.Name, Seen: now,
		}

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
				Resource: "pvc", PodName: pvc.PodName, Namespace: pvc.Namespace,
				Reason: "VolumeUsageHigh", Hint: fmt.Sprintf("VolumeUsage(%.0f%%)", pvc.UsagePercentage),
				Severity: severity, Owner: pvc.PVName,
			})
		} else if p.notifiedPvc[pvc.PVName] && pvc.UsagePercentage >= clear {
			currentNotified[pvc.PVName] = true
		}
	}

	if p.firstScan {
		p.firstScan = false
	}

	boundPV := make(map[string]bool, len(pvByPVC))
	for _, pvName := range pvByPVC {
		if pvName != "" {
			boundPV[pvName] = true
		}
	}

	if !incomplete {
		for pvName := range p.notifiedPvc {
			if currentNotified[pvName] {
				continue
			}
			switch {
			case seen[pvName]:
				// mounted this cycle and fell below clear → genuine resolve
				p.correlator.ResolveByResource("pvc", pvName)
				delete(p.lastUsage, pvName)
			case !boundPV[pvName]:
				// PVC deleted → genuine resolve + evict
				p.correlator.ResolveByResource("pvc", pvName)
				delete(p.lastUsage, pvName)
			default:
				// bound but unmounted → usage is static; keep firing on the
				// still-accurate sample (resolves only when re-mounted < clear,
				// or the PVC is deleted, above).
				if s, ok := p.lastUsage[pvName]; ok && s.Pct >= clear {
					currentNotified[pvName] = true
				} else {
					p.correlator.ResolveByResource("pvc", pvName)
					delete(p.lastUsage, pvName)
				}
			}
		}
	}

	if !incomplete {
		p.notifiedPvc = currentNotified
	} else {
		for k := range currentNotified {
			p.notifiedPvc[k] = true
		}
	}
}
