package crd

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

	"github.com/abahmed/kwatch/internal/inventory/kube/dynamicwatch"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/metrics"
)

func (w *Watcher) listCRD(
	ctx context.Context,
	dc dynamic.Interface,
) (*unstructured.UnstructuredList, error) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// No limit: the single-object check needs to see a second KwatchConfig.
	return dc.Resource(gvr).Namespace(w.namespace).List(
		listCtx, metav1.ListOptions{},
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

const (
	crdPollInterval  = time.Minute
	crdRetryBase     = time.Second
	crdRetryMaxDelay = time.Minute
)

// crdDelays are the waits between CRD polls and between start retries.
type crdDelays struct {
	poll, retryBase, retryMax time.Duration
}

func defaultCRDDelays() crdDelays {
	return crdDelays{
		poll: crdPollInterval, retryBase: crdRetryBase,
		retryMax: crdRetryMaxDelay,
	}
}

// retry is the optional-component backoff (1s, 2s, 4s, ... capped) for the
// n-th consecutive failed start, counting from 1.
func (d crdDelays) retry(failures int) time.Duration {
	delay := d.retryBase
	for i := 1; i < failures && delay < d.retryMax; i++ {
		delay *= 2
	}
	return min(delay, d.retryMax)
}

func (w *Watcher) waitForCRD(ctx context.Context, dc dynamic.Interface) {
	delays := w.delays
	if delays.poll == 0 {
		delays = defaultCRDDelays()
	}
	wait, failures := delays.poll, 0
	for {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		switch w.checkInstalledCRD(ctx, dc) {
		case crdRestarting:
			return
		case crdStarted:
			<-ctx.Done()
			return
		case crdFailed:
			failures++
			wait = delays.retry(failures)
		default:
			failures, wait = 0, delays.poll
		}
	}
}

// crdCheck is the outcome of one poll for the CRD.
type crdCheck int

const (
	// crdPending: the CRD is not installed yet; keep polling.
	crdPending crdCheck = iota
	// crdRestarting: late configuration asked for a restart.
	crdRestarting
	// crdStarted: the informer is running.
	crdStarted
	// crdFailed: the CRD exists but the informer did not start; retry
	// with backoff so later KwatchConfig edits are not lost.
	crdFailed
)

// checkInstalledCRD polls once for the CRD and, if it has appeared,
// starts the informer.
func (w *Watcher) checkInstalledCRD(
	ctx context.Context, dc dynamic.Interface,
) crdCheck {
	list, err := w.listCRD(ctx, dc)
	if err != nil {
		if !isMissingCRD(err) {
			klog.V(2).InfoS(
				"CRD watcher discovery unavailable", "error", err,
			)
			w.reportError(err)
		}
		return crdPending
	}
	items := make([]interface{}, 0, len(list.Items))
	for i := range list.Items {
		items = append(items, &list.Items[i])
	}
	if w.restartForLateObjects(items) {
		return crdRestarting
	}
	if err := w.startInformer(ctx, dc, true); err != nil {
		klog.ErrorS(err, "CRD watcher failed to start after installation")
		w.reportError(err)
		return crdFailed
	}
	w.reportError(nil)
	return crdStarted
}

func isMissingCRD(err error) bool {
	return errors.IsNotFound(err)
}

// runInformer runs inf under the watcher's lifecycle wait group. The
// returned cancel stops it and done closes once it has returned.
func (w *Watcher) runInformer(
	ctx context.Context, inf cache.SharedIndexInformer,
) (context.CancelFunc, <-chan struct{}, error) {
	w.mu.Lock()
	runWG := w.runWG
	w.mu.Unlock()
	if runWG == nil {
		return nil, nil, fmt.Errorf("crdwatch: lifecycle is not active")
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
	return informerCancel, informerDone, nil
}

// eventHandler wraps the callbacks in SafeEventHandler, which also unwraps
// delete tombstones so a deletion after a watch gap is not missed.
func (w *Watcher) eventHandler() cache.ResourceEventHandler {
	return kubeclient.SafeEventHandler(
		"crdwatch", gvr.String(), cache.ResourceEventHandlerFuncs{
			AddFunc:    w.changed,
			UpdateFunc: func(_, obj interface{}) { w.changed(obj) },
			DeleteFunc: w.deleted,
		})
}

func (w *Watcher) startInformer(
	ctx context.Context,
	dc dynamic.Interface,
	restartOnInitial bool,
) error {
	_, inf, err := dynamicwatch.NewInformer(
		dc, w.resync, w.namespace, gvr, kubeclient.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("crdwatch: create informer: %w", err)
	}

	if _, err := inf.AddEventHandler(w.eventHandler()); err != nil {
		return fmt.Errorf("crdwatch: failed to register event handler: %w", err)
	}

	informerCancel, informerDone, err := w.runInformer(ctx, inf)
	if err != nil {
		return err
	}
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
		w.restartForLateObjects(initial)
	}

	klog.InfoS("CRD watcher started", "namespace", w.namespace)
	return nil
}
