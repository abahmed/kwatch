package controller

import (
	"context"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/resource"
)

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	for _, p := range c.allPipelines() {
		defer p.shutdown()
	}

	klog.InfoS("starting controller")

	klog.InfoS("waiting for informer caches to sync")
	var syncFns []cache.InformerSynced
	for _, p := range c.allPipelines() {
		syncFns = append(syncFns, p.synced...)
	}
	syncFns = append(syncFns, c.rsSynced...)
	syncFns = append(syncFns, c.dsSynced...)
	syncFns = append(syncFns, c.ssSynced...)
	syncFns = append(syncFns, c.eventsSynced...)
	syncFns = append(syncFns, c.configMapSynced...)
	syncFns = append(syncFns, c.secretsSynced...)
	syncFns = append(syncFns, c.graphSynced...)
	if err := c.waitForCaches(ctx, syncFns); err != nil {
		return err
	}
	if c.readyFn != nil {
		c.readyFn()
	}

	c.buildGraph()
	c.recordGraphSize()
	go func() {
		rebuildTicker := time.NewTicker(60 * time.Minute)
		defer rebuildTicker.Stop()
		pruneTicker := time.NewTicker(5 * time.Minute)
		defer pruneTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-rebuildTicker.C:
				c.buildGraph()
				c.recordGraphSize()
			case <-pruneTicker.C:
				c.pruneGraph()
				c.recordGraphSize()
			}
		}
	}()
	c.buildSeenSet()
	if c.cpPod.startWorkers {
		c.handler.SweepControlPlane()
	}

	if c.nodeResourceCfg != nil {
		go func(cfg *config.NodeResourceMonitor) {
			interval := time.Duration(cfg.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 300 * time.Second
			}
			mon := resource.NewMonitor(resource.Config{
				Interval:   interval,
				CpuWarning: cfg.CpuWarning, CpuCritical: cfg.CpuCritical,
				MemWarning: cfg.MemWarning, MemCritical: cfg.MemCritical,
				FilesystemWarningPercent:  cfg.FilesystemWarningPercent,
				FilesystemCriticalPercent: cfg.FilesystemCriticalPercent,
				InodeWarningPercent:       cfg.InodeWarningPercent,
				InodeCriticalPercent:      cfg.InodeCriticalPercent,
				Client:                    c.client,
			}, c.nodeLister, c.podLister)
			mon.Run(ctx, func(sig *event.Signal) {
				c.handler.ProcessNodeResourceOvercommit(
					sig.Reason,
					sig.NodeName,
					sig.Hint,
					sig.Severity,
				)
			})
		}(c.nodeResourceCfg)
	}

	klog.InfoS("starting workers")
	for i := 0; i < workers; i++ {
		for _, p := range c.activePipelines() {
			go wait.UntilWithContext(ctx, p.worker, time.Second)
		}
	}

	<-ctx.Done()
	klog.InfoS("shutting down workers")
	return nil
}
