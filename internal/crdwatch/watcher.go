package crdwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/k8s"
)

var gvr = schema.GroupVersionResource{
	Group:    "kwatch.abahmed.dev",
	Version:  "v1alpha1",
	Resource: "kwatchconfigs",
}

// Watcher monitors KwatchConfig CRs. Config changes are deliberately not
// applied live: a partially reconfigured controller is harder to reason about
// than a brief, explicit restart.

type Watcher struct {
	cfg         *config.Config
	restConfig  *rest.Config
	namespace   string
	resync      time.Duration
	mu          sync.Mutex
	seen        map[string]string
	ready       bool
	restart     func()
	restartOnce sync.Once
}

func New(cfg *config.Config, restConfig *rest.Config, namespace string, resync time.Duration, restart func()) *Watcher {
	return &Watcher{
		cfg: cfg, restConfig: restConfig, namespace: namespace,
		resync: resync, seen: make(map[string]string), restart: restart,
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	if !w.cfg.CrdConfig.Enabled {
		klog.V(4).InfoS("CRD watcher is disabled")
		return nil
	}

	dc, err := dynamic.NewForConfig(w.restConfig)
	if err != nil {
		return fmt.Errorf("crdwatch: failed to create dynamic client: %w", err)
	}

	// Pre-flight: check if the CRD is installed. If it is installed later,
	// keep watching for it instead of requiring a process restart.
	if _, err := dc.Resource(gvr).Namespace(w.namespace).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("CRD kwatchconfigs.kwatch.abahmed.dev not found; waiting for installation")
			go w.waitForCRD(ctx, dc)
			return nil
		}
		return fmt.Errorf("crdwatch: preflight check failed: %w", err)
	}
	return w.startInformer(ctx, dc, false)
}

func (w *Watcher) waitForCRD(ctx context.Context, dc dynamic.Interface) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			list, err := dc.Resource(gvr).Namespace(w.namespace).List(
				ctx, metav1.ListOptions{Limit: 1},
			)
			if err != nil {
				if !errors.IsNotFound(err) {
					klog.V(2).InfoS("CRD watcher discovery unavailable", "error", err)
				}
				continue
			}
			if w.restartForLateConfig(len(list.Items)) {
				return
			}
			if err := w.startInformer(ctx, dc, true); err != nil {
				klog.ErrorS(err, "CRD watcher failed to start after installation")
			}
			return
		}
	}
}

func (w *Watcher) startInformer(
	ctx context.Context,
	dc dynamic.Interface,
	restartOnInitial bool,
) error {

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dc, w.resync, w.namespace, nil)
	inf := factory.ForResource(gvr).Informer()
	if err := inf.SetTransform(k8s.TrimManagedFields); err != nil {
		return fmt.Errorf("crdwatch: set cache transform: %w", err)
	}

	if _, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.changed,
		UpdateFunc: func(_, obj interface{}) { w.changed(obj) },
		DeleteFunc: w.deleted,
	}); err != nil {
		return fmt.Errorf("crdwatch: failed to register event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
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

func (w *Watcher) restartForLateConfig(count int) bool {
	if count == 0 || w.restart == nil {
		return false
	}
	w.restartOnce.Do(func() {
		klog.InfoS(
			"KwatchConfig appeared after startup; restarting to apply configuration",
		)
		w.restart()
	})
	return true
}

func (w *Watcher) seedKnown(objects []interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, obj := range objects {
		if accessor, err := meta.Accessor(obj); err == nil {
			w.seen[accessor.GetNamespace()+"/"+accessor.GetName()] = accessor.GetResourceVersion()
		}
	}
	w.ready = true
}

func (w *Watcher) changed(obj interface{}) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	key := accessor.GetNamespace() + "/" + accessor.GetName()
	version := accessor.GetResourceVersion()
	w.mu.Lock()
	previous, known := w.seen[key]
	if !known {
		w.seen[key] = version
	}
	ready := w.ready
	w.mu.Unlock()
	if !ready || previous == version || w.restart == nil {
		return
	}
	w.restartOnce.Do(func() {
		klog.InfoS("KwatchConfig changed; restarting to apply configuration")
		w.restart()
	})
}

func (w *Watcher) deleted(obj interface{}) {
	accessor, err := meta.Accessor(obj)
	if err != nil || w.restart == nil {
		return
	}
	key := accessor.GetNamespace() + "/" + accessor.GetName()
	w.mu.Lock()
	_, known := w.seen[key]
	delete(w.seen, key)
	ready := w.ready
	w.mu.Unlock()
	if !ready || !known {
		return
	}
	w.restartOnce.Do(func() {
		klog.InfoS("KwatchConfig changed; restarting to apply configuration")
		w.restart()
	})
}
