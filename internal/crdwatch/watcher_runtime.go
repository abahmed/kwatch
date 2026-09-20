package crdwatch

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
	"github.com/abahmed/kwatch/internal/metrics"
)

func (w *Watcher) listCRD(
	ctx context.Context,
	dc dynamic.Interface,
) (*unstructured.UnstructuredList, error) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return dc.Resource(gvr).Namespace(w.namespace).List(
		listCtx, metav1.ListOptions{Limit: 1},
	)
}

func (w *Watcher) runWaitingGeneration(
	ctx context.Context,
	dc dynamic.Interface,
	generation uint64,
) {
	defer func() {
		w.mu.Lock()
		runWG := w.runWG
		w.mu.Unlock()
		if runWG != nil {
			runWG.Wait()
		}
		w.resetLifecycle(generation)
	}()
	w.waitForCRD(ctx, dc)
}

func (w *Watcher) waitForCRD(ctx context.Context, dc dynamic.Interface) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			list, err := w.listCRD(ctx, dc)
			if err != nil {
				if !isMissingCRD(err) {
					klog.V(2).InfoS(
						"CRD watcher discovery unavailable", "error", err,
					)
					w.reportError(err)
				}
				continue
			}
			if w.restartForLateConfig(len(list.Items)) {
				return
			}
			if err := w.startInformer(ctx, dc, true); err != nil {
				klog.ErrorS(err, "CRD watcher failed to start after installation")
				w.reportError(err)
			} else {
				w.reportError(nil)
			}
			<-ctx.Done()
			return
		}
	}
}

func isMissingCRD(err error) bool {
	return errors.IsNotFound(err)
}

func (w *Watcher) startInformer(
	ctx context.Context,
	dc dynamic.Interface,
	restartOnInitial bool,
) error {
	_, inf, err := dynamicwatch.NewInformer(
		dc, w.resync, w.namespace, gvr, k8s.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("crdwatch: create informer: %w", err)
	}

	if _, err := inf.AddEventHandler(k8s.SafeEventHandler(
		"crdwatch", gvr.String(), cache.ResourceEventHandlerFuncs{
			AddFunc:    w.changed,
			UpdateFunc: func(_, obj interface{}) { w.changed(obj) },
			DeleteFunc: w.deleted,
		})); err != nil {
		return fmt.Errorf("crdwatch: failed to register event handler: %w", err)
	}

	w.mu.Lock()
	runWG := w.runWG
	w.mu.Unlock()
	if runWG == nil {
		return fmt.Errorf("crdwatch: lifecycle is not active")
	}
	informerCtx, informerCancel := context.WithCancel(ctx)
	informerDone := make(chan struct{})
	runWG.Add(1)
	go func() {
		defer runWG.Done()
		defer close(informerDone)
		defer informerCancel()
		inf.Run(informerCtx.Done())
	}()
	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	metrics.DefaultRegistry().WatcherSyncs.Add(1)
	if !cache.WaitForCacheSync(syncCtx.Done(), inf.HasSynced) {
		metrics.DefaultRegistry().WatcherSyncFailures.Add(1)
		informerCancel()
		<-informerDone
		return fmt.Errorf("crdwatch: failed to sync informer cache")
	}

	initial := inf.GetStore().List()
	w.seedKnown(initial)
	if restartOnInitial {
		w.restartForLateConfig(len(initial))
	}

	klog.InfoS("CRD watcher started", "namespace", w.namespace)
	return nil
}
