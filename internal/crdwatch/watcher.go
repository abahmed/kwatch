package crdwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/api/v1alpha1"
	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

var gvr = schema.GroupVersionResource{
	Group:    "kwatch.abahmed.dev",
	Version:  "v1alpha1",
	Resource: "kwatchconfigs",
}

// Watcher monitors KwatchConfig CRs and applies safe config changes live.
type SeveritySetter interface {
	SetSeverityMap(map[string]string)
}

type Watcher struct {
	cfg           *config.Config
	alertManager  *alert.AlertManager
	engine        SeveritySetter
	restConfig    *rest.Config
	namespace     string
	resync        time.Duration
	mu            sync.Mutex
	fallbackCfg   *config.Config // boot-time values restored on CR delete
}

func New(cfg *config.Config, alertManager *alert.AlertManager, engine SeveritySetter, restConfig *rest.Config, namespace string, resync time.Duration) *Watcher {
	return &Watcher{
		cfg:          cfg,
		alertManager: alertManager,
		engine:       engine,
		restConfig:   restConfig,
		namespace:    namespace,
		resync:       resync,
		fallbackCfg:  cfg,
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

	// Pre-flight: check if the CRD is installed
	if _, err := dc.Resource(gvr).Namespace(w.namespace).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("CRD kwatchconfigs.kwatch.abahmed.dev not found — CRD watcher skipped")
			return nil
		}
		return fmt.Errorf("crdwatch: preflight check failed: %w", err)
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dc, w.resync, w.namespace, nil)
	inf := factory.ForResource(gvr).Informer()

	inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.reload(obj) },
		UpdateFunc: func(_, newObj interface{}) { w.reload(newObj) },
		DeleteFunc: func(_ interface{}) { w.restore() },
	})

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return fmt.Errorf("crdwatch: failed to sync informer cache")
	}

	// Process existing CRs
	items, err := dc.Resource(gvr).Namespace(w.namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range items.Items {
			w.reload(&items.Items[i])
		}
	}

	klog.InfoS("CRD watcher started", "namespace", w.namespace)
	return nil
}

// reload converts an unstructured KwatchConfig CR into typed config and
// hot-applies safe fields. Restart-only fields are logged and skipped.
func (w *Watcher) reload(obj interface{}) {
	unstr, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	var cr v1alpha1.KwatchConfig
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.UnstructuredContent(), &cr); err != nil {
		klog.ErrorS(err, "crdwatch: failed to convert unstructured to KwatchConfig", "name", unstr.GetName())
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	spec := cr.Spec

	if spec.MaxRecentLogLines > 0 {
		w.alertManager.SetMaxLogLines(int(spec.MaxRecentLogLines))
	}

	{
		silences := make([]config.SilenceRule, 0, len(spec.Silences))
		for _, s := range spec.Silences {
			silences = append(silences, config.SilenceRule{
				Namespaces:      s.Namespaces,
				Reasons:         s.Reasons,
				PodNamePatterns: s.PodNamePatterns,
			})
		}
		w.alertManager.SetSilences(silences)
	}

	if spec.SeverityByOwnerKind != nil {
		w.engine.SetSeverityMap(spec.SeverityByOwnerKind)
	}

	// Log restart-only fields that can't be hot-applied
	if spec.Workers > 0 || spec.PvcMonitor.Enabled || spec.NodeMonitor.Enabled || spec.RolloutMonitor.Enabled || spec.DaemonSetMonitor.Enabled || spec.JobMonitor.Enabled || spec.CronJobMonitor.Enabled {
		klog.InfoS("crdwatch: some config changes require a restart to take effect",
			"crd", cr.Name)
	}

	klog.V(4).InfoS("crdwatch: applied config from CR", "name", cr.Name)
}

// restore re-applies the boot-time ConfigMap-derived config on CR deletion.
func (w *Watcher) restore() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.alertManager.SetSilences(w.fallbackCfg.Silences)
	if w.fallbackCfg.MaxRecentLogLines > 0 {
		w.alertManager.SetMaxLogLines(int(w.fallbackCfg.MaxRecentLogLines))
	}
	w.engine.SetSeverityMap(w.fallbackCfg.SeverityByOwnerKind)

	klog.InfoS("crdwatch: restored config from boot-time snapshot")
}
