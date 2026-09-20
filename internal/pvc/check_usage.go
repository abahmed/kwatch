package pvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// checkUsage queries ready nodes for mounted-volume usage and combines the
// results into one complete sweep.
func (p *PvcMonitor) checkUsage(ctx context.Context) {
	p.checkVolumeStatus(ctx)
	nodes, err := k8s.GetNodes(ctx, p.client)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: failed to get nodes")
		return
	}

	nodeNames := make([]string, 0)
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if k8s.IsNodeReady(node) {
			nodeNames = append(nodeNames, node.Name)
		}
	}
	pvByPVC := p.pvcMap(ctx)

	type nodeResult struct {
		usages []*PvcUsage
		err    error
	}
	results := make([]nodeResult, len(nodeNames))
	var waitGroup sync.WaitGroup
	canceled := false
	for i, nodeName := range nodeNames {
		select {
		case p.sem <- struct{}{}:
		case <-ctx.Done():
			canceled = true
		}
		if canceled {
			break
		}
		waitGroup.Add(1)
		go func(index int, name string) {
			defer waitGroup.Done()
			defer func() { <-p.sem }()
			usage, usageErr := p.getNodeUsage(ctx, name, pvByPVC)
			results[index] = nodeResult{usage, usageErr}
		}(i, nodeName)
	}
	waitGroup.Wait()
	if canceled || ctx.Err() != nil {
		return
	}

	var usages []*PvcUsage
	incomplete := false
	for _, result := range results {
		if result.err != nil {
			klog.ErrorS(result.err, "pvc monitor: node usage failed")
			incomplete = true
			continue
		}
		usages = append(usages, result.usages...)
	}
	p.apply(usages, pvByPVC, incomplete, true)
}

func (p *PvcMonitor) checkVolumeStatus(ctx context.Context) {
	if p.client == nil {
		return
	}
	pvcs, err := p.listPVCs(ctx)
	if err != nil {
		klog.ErrorS(err, "pvc monitor: failed to list pvc status")
		return
	}
	for i := range pvcs {
		pvc := &pvcs[i]
		if !p.namespaceAllowed(pvc.Namespace) {
			continue
		}
		key := pvc.Namespace + "/" + pvc.Name
		condition := pvcFailureCondition(pvc.Status)
		stuck := volumeStuckTerminating(
			pvc.DeletionTimestamp, pvc.Finalizers, p.now(),
		)
		if pvcStatusFailure(pvc.Status.Phase) || condition != "" || stuck {
			hint := fmt.Sprintf("PVC %s is %s", key, pvc.Status.Phase)
			if condition != "" {
				hint = fmt.Sprintf("PVC %s has storage condition %s", key, condition)
			}
			if stuck {
				hint = fmt.Sprintf(
					"PVC %s has been terminating for %s with finalizers: %v",
					key, p.now().Sub(pvc.DeletionTimestamp.Time).Round(time.Minute),
					pvc.Finalizers,
				)
			}
			p.report(observe.Object(
				"pvc", pvc, constant.ReasonPersistentVolumeClaim,
			).WithHint(hint))
		} else {
			p.resolve(model.NewObjectRef("pvc", pvc.Namespace, pvc.Name))
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
		stuck := volumeStuckTerminating(
			pv.DeletionTimestamp, pv.Finalizers, p.now(),
		)
		if pvStatusFailure(pv.Status.Phase) || pv.Status.Reason != "" || stuck {
			hint := fmt.Sprintf("PV %s is %s", pv.Name, pv.Status.Phase)
			if pv.Status.Reason != "" || pv.Status.Message != "" {
				hint = fmt.Sprintf(
					"PV %s: %s", pv.Name,
					joinStatusDetails(pv.Status.Reason, pv.Status.Message),
				)
			}
			if stuck {
				hint = fmt.Sprintf(
					"PV %s has been terminating for %s with finalizers: %v",
					pv.Name, p.now().Sub(pv.DeletionTimestamp.Time).Round(time.Minute),
					pv.Finalizers,
				)
			}
			p.report(observe.ClusterObject(
				"pv", pv.Name, constant.ReasonPersistentVolume,
			).WithLabels(pv.Labels).WithHint(hint))
		} else {
			p.resolve(model.ObjectRef{Kind: "pv", Name: pv.Name})
		}
	}
}
