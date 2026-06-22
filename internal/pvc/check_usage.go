package pvc

import (
	"context"
	"fmt"
	"sync"
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
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if !k8s.IsNodeReady(node) {
			continue
		}
		nodeNames = append(nodeNames, node.Name)
	}

	pvByPVC := p.pvcMap(ctx)

	type nodeResult struct {
		usages []*PvcUsage
		err    error
	}
	results := make([]nodeResult, len(nodeNames))

	var wg sync.WaitGroup
	for i, nodeName := range nodeNames {
		p.sem <- struct{}{}
		wg.Add(1)
		go func(idx int, nn string) {
			defer wg.Done()
			defer func() { <-p.sem }()
			u, err := p.getNodeUsage(ctx, nn, pvByPVC)
			results[idx] = nodeResult{u, err}
		}(i, nodeName)
	}
	wg.Wait()

	var pvcUsages []*PvcUsage
	incomplete := false
	for _, r := range results {
		if r.err != nil {
			klog.ErrorS(r.err, "pvc monitor: node usage failed")
			incomplete = true
			continue
		}
		pvcUsages = append(pvcUsages, r.usages...)
	}

	p.apply(pvcUsages, pvByPVC, incomplete, true /*isSweep*/)
}

// SampleNode reads stats/summary for ONE node out-of-cycle and folds the result
// into the cache + correlator. Called from the pod informer when a tracked PVC's
// pod goes Running. Debounces to ≤1 kubelet read per node per window.
// Back-pressure: drops the sample when the concurrent-sample limit is reached.
func (p *PvcMonitor) SampleNode(ctx context.Context, nodeName string) {
	if !p.config.Enabled || nodeName == "" {
		return
	}

	// Fast path: debounce check first (no API call, no semaphore).
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

	// B9: bounded concurrency — drop if the burst limit is reached.
	select {
	case p.sem <- struct{}{}:
	default:
		klog.V(4).InfoS("pvc monitor: dropping SampleNode, burst limit reached", "node", nodeName)
		return
	}
	defer func() { <-p.sem }()

	// Skip NotReady nodes — their kubelet can't serve stats/summary anyway.
	if node, err := p.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{}); err == nil {
		if !k8s.IsNodeReady(node) {
			return
		}
	}

	pvByPVC := p.pvcMap(ctx)

	usages, err := p.getNodeUsage(ctx, nodeName, pvByPVC)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: SampleNode usage failed", "node", nodeName)
		return
	}

	// incomplete=true: single-node view is partial, so update+signal but DON'T
	// run cluster-wide resolves (those stay with the periodic full sweep).
	p.apply(usages, pvByPVC, true, false /*isSweep*/)
}

// apply folds one batch of observations into the cache + correlator under p.mu.
// Pure in-memory — no K8s writes. incomplete=true means "partial view" (single
// node / per-node error): update+signal but skip the cluster-wide unmounted/deleted
// resolve pass (only the full sweep owns resolves). isSweep=true for the periodic
// full sweep, false for event-driven SampleNode (only the sweep clears firstScan
// and the sweep re-signals unconditionally for edgeAction dedup).
func (p *PvcMonitor) apply(pvcUsages []*PvcUsage, pvByPVC map[string]string, incomplete bool, isSweep bool) {
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
		// B1: only cache entries that can keep an incident alive (>= clear)
		if pvc.UsagePercentage >= clear {
			p.lastUsage[pvc.PVName] = state.PvcSample{
				Pct: pvc.UsagePercentage, Namespace: pvc.Namespace,
				Name: pvc.Name, PodName: pvc.PodName, Seen: now,
			}
		} else {
			delete(p.lastUsage, pvc.PVName)
		}

		if pvc.UsagePercentage >= p.config.Threshold {
			wasNotified := p.notifiedPvc[pvc.PVName]
			currentNotified[pvc.PVName] = true
			if p.firstScan {
				continue
			}
			// B8: SampleNode (isSweep=false) only signals the rising edge;
			// the sweep re-signals unconditionally (edgeAction dedups).
			if isSweep || !wasNotified {
				severity := "normal"
				if pvc.UsagePercentage >= p.config.CriticalThreshold {
					severity = "high"
				}
				p.reportSignal(&event.Signal{
					Resource: "pvc", PodName: pvc.PodName, Namespace: pvc.Namespace,
					Reason: "VolumeUsageHigh", Hint: fmt.Sprintf("VolumeUsage(%.0f%%)", pvc.UsagePercentage),
					Severity: severity, Owner: pvc.PVName,
				})
			}
		} else if p.notifiedPvc[pvc.PVName] && pvc.UsagePercentage >= clear {
			currentNotified[pvc.PVName] = true
		}
	}

	// B5: only the full sweep clears firstScan (SampleNode must not consume it)
	if isSweep && p.firstScan {
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
