package kube

import (
	"context"
	"sort"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

// Dynamic source limits.
const (
	// DefaultMaxCustomResources caps watched custom resource types so a
	// cluster with hundreds of CRDs cannot exhaust kwatch's memory.
	DefaultMaxCustomResources = 150
	rediscoveryInterval       = 5 * time.Minute
)

var (
	crdResource = schema.GroupVersionResource{
		Group: "apiextensions.k8s.io", Version: "v1",
		Resource: "customresourcedefinitions",
	}
	apiServiceResource = schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1",
		Resource: "apiservices",
	}
)

// watchedResource is one API resource the dynamic source follows.
type watchedResource struct {
	gvr  schema.GroupVersionResource
	kind string
}

// DynamicConfig configures the dynamic source.
type DynamicConfig struct {
	Client dynamic.Interface
	Resync time.Duration
	Now    func() time.Time
	Submit Submit
	Max    int
}

// DynamicSource watches APIServices and every custom resource type that
// reports status, rediscovering CRDs periodically so resources installed
// after startup are watched too.
type DynamicSource struct {
	cfg     DynamicConfig
	mu      sync.Mutex
	running map[schema.GroupVersionResource]context.CancelFunc
	wg      sync.WaitGroup
}

// NewDynamicSource builds a dynamic source.
func NewDynamicSource(cfg DynamicConfig) *DynamicSource {
	if cfg.Max <= 0 {
		cfg.Max = DefaultMaxCustomResources
	}
	return &DynamicSource{
		cfg:     cfg,
		running: make(map[schema.GroupVersionResource]context.CancelFunc),
	}
}

// Run discovers and watches resources until ctx ends, then waits for every
// informer to stop.
func (d *DynamicSource) Run(ctx context.Context) {
	defer d.wg.Wait()
	ticker := time.NewTicker(rediscoveryInterval)
	defer ticker.Stop()
	for {
		d.reconcile(ctx)
		select {
		case <-ctx.Done():
			d.stopAll()
			return
		case <-ticker.C:
		}
	}
}

func (d *DynamicSource) reconcile(ctx context.Context) {
	desired := d.discover(ctx)
	d.mu.Lock()
	defer d.mu.Unlock()
	wanted := make(map[schema.GroupVersionResource]bool, len(desired))
	for _, r := range desired {
		wanted[r.gvr] = true
		if _, running := d.running[r.gvr]; !running {
			d.start(ctx, r)
		}
	}
	for gvr, cancel := range d.running {
		if !wanted[gvr] {
			cancel()
			delete(d.running, gvr)
		}
	}
}

// discover lists APIServices plus every served storage version of a CRD
// with a status subresource, capped at Max custom types.
func (d *DynamicSource) discover(ctx context.Context) []watchedResource {
	out := []watchedResource{{gvr: apiServiceResource, kind: "APIService"}}
	list, err := d.cfg.Client.Resource(crdResource).List(ctx,
		metav1.ListOptions{})
	if err != nil {
		klog.V(2).InfoS("custom resource discovery unavailable",
			"error", err)
		return out
	}
	var custom []watchedResource
	for i := range list.Items {
		if r, ok := statusResource(&list.Items[i]); ok {
			custom = append(custom, r)
		}
	}
	sort.Slice(custom, func(i, j int) bool {
		return custom[i].gvr.String() < custom[j].gvr.String()
	})
	if len(custom) > d.cfg.Max {
		klog.InfoS("custom resource types capped", "found", len(custom),
			"watched", d.cfg.Max)
		custom = custom[:d.cfg.Max]
	}
	return append(out, custom...)
}

func statusResource(crd *unstructured.Unstructured) (watchedResource, bool) {
	group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
	plural, _, _ := unstructured.NestedString(
		crd.Object, "spec", "names", "plural")
	kind, _, _ := unstructured.NestedString(
		crd.Object, "spec", "names", "kind")
	versions, _, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
	for _, raw := range versions {
		v, _ := raw.(map[string]any)
		served, _ := v["served"].(bool)
		storage, _ := v["storage"].(bool)
		name, _ := v["name"].(string)
		_, hasStatus, _ := unstructured.NestedMap(v, "subresources", "status")
		if served && storage && hasStatus && group != "" && plural != "" {
			return watchedResource{
				gvr: schema.GroupVersionResource{
					Group: group, Version: name, Resource: plural,
				},
				kind: kind,
			}, true
		}
	}
	return watchedResource{}, false
}

// start runs one informer; the caller holds d.mu.
func (d *DynamicSource) start(ctx context.Context, r watchedResource) {
	_, informer, err := dynamicwatch.NewInformer(d.cfg.Client,
		d.cfg.Resync, metav1.NamespaceAll, r.gvr, TrimToLatestManager)
	if err != nil {
		klog.ErrorS(err, "dynamic informer", "resource", r.gvr)
		return
	}
	translator := NewTranslator(NewUnstructuredSchema(r.kind))
	if _, err := informer.AddEventHandler(translatorHandler(
		translator, d.cfg.Submit, d.cfg.Now)); err != nil {
		klog.ErrorS(err, "dynamic informer handler", "resource", r.gvr)
		return
	}
	resourceCtx, cancel := context.WithCancel(ctx)
	d.running[r.gvr] = cancel
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		informer.Run(resourceCtx.Done())
	}()
}

func (d *DynamicSource) stopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for gvr, cancel := range d.running {
		cancel()
		delete(d.running, gvr)
	}
}
