package pvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const stuckVolumeDeletionGrace = 10 * time.Minute

// checkUsage iterates all nodes and queries the kubelet summary API for
// volume usage. Only PVCs that are actively mounted by a pod on the node
// appear in the summary. PVCs that are Bound but not yet mounted (e.g. a
// newly created PVC whose consumer pod hasn't scheduled) are invisible to
// this check and will not trigger alerts until a pod mounts them.
func (p *PvcMonitor) checkUsage(ctx context.Context) {
	// Mounted-volume usage is only one storage signal. The API status is the
	// authoritative signal for Pending/Lost PVCs and Released/Failed PVs, so
	// inspect it before querying kubelet summaries. This also works on clusters
	// with no ready nodes.
	p.checkVolumeStatus(ctx)

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

func (p *PvcMonitor) checkVolumeStatus(ctx context.Context) {
	if p.client == nil {
		return
	}
	pvcs, err := p.client.CoreV1().PersistentVolumeClaims("").List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: failed to list pvc status")
		return
	}
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		if !p.namespaceAllowed(pvc.Namespace) {
			continue
		}
		key := pvc.Namespace + "/" + pvc.Name
		condition := pvcFailureCondition(pvc.Status)
		stuck := volumeStuckTerminating(pvc.DeletionTimestamp, pvc.Finalizers, p.now())
		if pvcStatusFailure(pvc.Status.Phase) || condition != "" || stuck {
			hint := fmt.Sprintf("PVC %s is %s", key, pvc.Status.Phase)
			if condition != "" {
				hint = fmt.Sprintf("PVC %s has storage condition %s", key, condition)
			}
			if stuck {
				hint = fmt.Sprintf("PVC %s has been terminating for %s with finalizers: %v",
					key, p.now().Sub(pvc.DeletionTimestamp.Time).Round(time.Minute), pvc.Finalizers)
			}
			p.reportSignal(&event.Signal{
				Resource: "pvc", Namespace: pvc.Namespace, PodName: pvc.Name,
				Owner: key, Reason: constant.ReasonPersistentVolumeClaim,
				Labels: pvc.Labels,
				Hint:   hint,
			})
		} else {
			p.correlator.ResolveByResource("pvc", key)
		}
	}

	pvs, err := p.client.CoreV1().PersistentVolumes().List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: failed to list pv status")
		return
	}
	for i := range pvs.Items {
		pv := &pvs.Items[i]
		stuck := volumeStuckTerminating(pv.DeletionTimestamp, pv.Finalizers, p.now())
		if pvStatusFailure(pv.Status.Phase) || pv.Status.Reason != "" || stuck {
			hint := fmt.Sprintf("PV %s is %s", pv.Name, pv.Status.Phase)
			if pv.Status.Reason != "" || pv.Status.Message != "" {
				hint = fmt.Sprintf("PV %s: %s", pv.Name, joinStatusDetails(pv.Status.Reason, pv.Status.Message))
			}
			if stuck {
				hint = fmt.Sprintf("PV %s has been terminating for %s with finalizers: %v",
					pv.Name, p.now().Sub(pv.DeletionTimestamp.Time).Round(time.Minute), pv.Finalizers)
			}
			p.reportSignal(&event.Signal{
				Resource: "pv", PodName: pv.Name, Owner: pv.Name,
				Reason: constant.ReasonPersistentVolume, Labels: pv.Labels,
				Hint: hint,
			})
		} else {
			p.correlator.ResolveByResource("pv", pv.Name)
		}
	}
}

func volumeStuckTerminating(deletion *metav1.Time, finalizers []string, now time.Time) bool {
	return deletion != nil && len(finalizers) > 0 &&
		now.Sub(deletion.Time) >= stuckVolumeDeletionGrace
}

func pvcStatusFailure(phase corev1.PersistentVolumeClaimPhase) bool {
	return phase == corev1.ClaimPending || phase == corev1.ClaimLost
}

func pvcFailureCondition(status corev1.PersistentVolumeClaimStatus) string {
	for resourceName, resizeStatus := range status.AllocatedResourceStatuses {
		switch resizeStatus {
		case corev1.PersistentVolumeClaimControllerResizeInfeasible,
			corev1.PersistentVolumeClaimNodeResizeInfeasible:
			return fmt.Sprintf("%s=%s", resourceName, resizeStatus)
		}
	}
	if status.ModifyVolumeStatus != nil &&
		status.ModifyVolumeStatus.Status == corev1.PersistentVolumeClaimModifyVolumeInfeasible {
		return fmt.Sprintf("ModifyVolume=%s", status.ModifyVolumeStatus.Status)
	}
	for _, condition := range status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case corev1.PersistentVolumeClaimControllerResizeError,
			corev1.PersistentVolumeClaimNodeResizeError,
			corev1.PersistentVolumeClaimVolumeModifyVolumeError,
			corev1.PersistentVolumeClaimFileSystemResizePending:
			return joinStatusDetails(string(condition.Type), condition.Message)
		}
	}
	return ""
}

func joinStatusDetails(reason, message string) string {
	if reason == "" {
		return message
	}
	if message == "" {
		return reason
	}
	return reason + ": " + message
}

func pvStatusFailure(phase corev1.PersistentVolumePhase) bool {
	return phase == corev1.VolumeReleased || phase == corev1.VolumeFailed
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
