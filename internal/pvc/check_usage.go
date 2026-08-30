package pvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/state"
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

// apply folds one batch of observations into the cache + correlator under p.mu.
// Pure in-memory — no K8s writes. incomplete=true means "partial view" (single
// node / per-node error): update+signal but skip the cluster-wide unmounted/deleted
// resolve pass (only the full sweep owns resolves). isSweep=true for the periodic
// full sweep (only the sweep clears firstScan and the sweep re-signals
// unconditionally for edgeAction dedup).
// effectiveClear returns the clear threshold clamped into (0, Threshold].
func (p *PvcMonitor) effectiveClear() float64 {
	clear := p.config.ClearThreshold
	if clear <= 0 || clear > p.config.Threshold {
		clear = p.config.Threshold
	}
	return clear
}

// cacheSample refreshes the last-seen usage entry for PVs that can keep an
// incident alive and evicts entries that fell below the clear level.
func (p *PvcMonitor) cacheSample(now time.Time, u *PvcUsage, clear float64) {
	if u.UsagePercentage >= clear {
		p.lastUsage[u.PVName] = state.PvcSample{
			Pct: u.UsagePercentage, Namespace: u.Namespace,
			Name: u.Name, PodName: u.PodName, Seen: now,
		}
	} else {
		delete(p.lastUsage, u.PVName)
	}
}

// signalIfOver reports a volume-usage incident for an above-threshold sample.
// B8: SampleNode (isSweep=false) only signals the rising edge; the sweep
// re-signals unconditionally (edgeAction dedups).
func (p *PvcMonitor) signalIfOver(u *PvcUsage, isSweep bool, currentNotified map[string]bool) {
	wasNotified := p.notifiedPvc[u.PVName]
	currentNotified[u.PVName] = true
	if p.firstScan {
		return
	}
	if isSweep || !wasNotified {
		severity := model.SeverityNormal
		if u.UsagePercentage >= p.config.CriticalThreshold {
			severity = model.SeverityHigh
		}
		p.reportSignal(&event.Signal{
			Resource: "pvc", PodName: u.PodName, Namespace: u.Namespace,
			Reason: constant.ReasonVolumeUsageHigh, Hint: fmt.Sprintf("VolumeUsage(%.0f%%)", u.UsagePercentage),
			Severity: severity, Owner: u.PVName,
		})
	}
}

// resolveStale resolves or retains incidents for PVs no longer over threshold.
func (p *PvcMonitor) resolveStale(seen, boundPV, currentNotified map[string]bool, clear float64) {
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

func (p *PvcMonitor) apply(pvcUsages []*PvcUsage, pvByPVC map[string]string, incomplete bool, isSweep bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	clear := p.effectiveClear()

	currentNotified := make(map[string]bool, len(pvcUsages))
	seen := make(map[string]bool, len(pvcUsages))

	for _, pvc := range pvcUsages {
		seen[pvc.PVName] = true
		p.cacheSample(now, pvc, clear)

		if pvc.UsagePercentage >= p.config.Threshold {
			p.signalIfOver(pvc, isSweep, currentNotified)
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
		p.resolveStale(seen, boundPV, currentNotified, clear)
	}

	if !incomplete {
		p.notifiedPvc = currentNotified
	} else {
		for k := range currentNotified {
			p.notifiedPvc[k] = true
		}
	}
}
