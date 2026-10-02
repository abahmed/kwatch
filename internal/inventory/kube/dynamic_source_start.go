package kube

import (
	"context"
	"fmt"
	"reflect"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube/dynamicwatch"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/metrics"
)

// start runs one informer in the resource's watch mode and reports
// whether it started; the caller holds d.mu. A previous watch of the
// same type (prev, may be nil) hands over its admitted objects: the new
// informer runs once prev has stopped, and objects that disappeared
// between the two are reported gone once it has synced.
func (d *DynamicSource) start(
	ctx context.Context, r plannedResource, prev *dynamicWatch,
) bool {
	admission := d.admissionFor(r, prev)
	informer, sch, err := d.informerFor(r, admission)
	if err != nil {
		klog.V(2).InfoS("dynamic informer unavailable",
			"component", "inventory", "operation", "watch",
			"resource", r.gvr.String(), "mode", r.mode, "error", err)
		return false
	}
	resourceCtx, cancel := context.WithCancel(ctx)
	w := &dynamicWatch{
		kind: KindFor(r.kind), mode: r.mode, parent: ctx, cancel: cancel,
		done:  make(chan struct{}),
		store: informer.GetStore(),
		translator: NewTranslator(withGenericAttributes(sch, r.mode)).
			WithMaintenance(d.cfg.Maintenance),
		admission: admission,
	}
	if prev != nil {
		w.translator = prev.translator
		w.handedOver = make(chan struct{})
	}
	if err := d.addHandlers(resourceCtx, r, informer, w); err != nil {
		cancel()
		klog.ErrorS(err, "dynamic informer handler",
			"component", "inventory", "resource", r.gvr.String())
		return false
	}
	d.running[r.gvr] = w
	// One cache synchronization attempt per started watch; a watch the
	// API refuses before it synced counts as a failure (see refuse).
	metrics.DefaultRegistry().WatcherSyncs.Add(1)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer close(w.done)
		if prev != nil && !stopped(resourceCtx, prev.done) {
			return
		}
		informer.Run(resourceCtx.Done())
	}()
	if prev != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.takeOver(resourceCtx, w)
		}()
	}
	return true
}

// admissionFor returns the object admission of a new watch: the previous
// watch's, so a restart keeps what it admitted, or a fresh one.
func (d *DynamicSource) admissionFor(
	r plannedResource, prev *dynamicWatch,
) *objectAdmission {
	if prev != nil {
		return prev.admission
	}
	return newAdmission(r.gvr, d.cfg.MaxObjectsPerKind, d.budget)
}

// stopped waits for done and reports false when ctx ends first.
func stopped(ctx context.Context, done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// informerFor builds the informer and schema of a watch mode. Its
// transform applies the object caps, so objects past a cap are cached
// only as stubs (see capTransform).
func (d *DynamicSource) informerFor(
	r plannedResource, admission *objectAdmission,
) (cache.SharedIndexInformer, Schema, error) {
	switch r.mode {
	case WatchMetadata:
		informer, err := dynamicwatch.NewMetadataInformer(d.cfg.Metadata,
			d.cfg.Resync, r.gvr, capTransform(admission, metadataTransform))
		return informer, NewMetadataSchema(r.gvr.Group, r.kind), err
	case WatchStatus:
		_, informer, err := dynamicwatch.NewInformer(d.cfg.Client,
			d.cfg.Resync, metav1.NamespaceAll, r.gvr,
			capTransform(admission, statusTransform))
		return informer, NewUnstructuredSchema(r.gvr.Group, r.kind), err
	}
	return nil, nil, fmt.Errorf("watch mode %q is not dynamic", r.mode)
}

func (d *DynamicSource) addHandlers(
	ctx context.Context, r plannedResource,
	informer cache.SharedIndexInformer, w *dynamicWatch,
) error {
	// A panicking translator must not stop the informer from delivering
	// later updates; SafeEventHandler counts and logs it.
	handle, err := informer.AddEventHandler(kubeclient.SafeEventHandler(
		"inventory", r.gvr.Resource,
		cappedHandler(ctx, w, d.cfg.Submit, d.cfg.Now)))
	if err != nil {
		return err
	}
	// The kind is synced once the handler, not only the informer's
	// store, has seen the initial list: only then does the model hold it.
	w.hasSynced = handle.HasSynced
	if r.tier == tierAnchor {
		if _, err := informer.AddEventHandler(kubeclient.SafeEventHandler(
			"inventory", r.gvr.Resource,
			rediscoveryHandler(d.requestDiscovery))); err != nil {
			return err
		}
	}
	gvr := r.gvr
	// The handler names its own watch: a reflector that is still
	// stopping after a restart must not refuse its replacement.
	return informer.SetWatchErrorHandlerWithContext(
		func(ctx context.Context, reflector *cache.Reflector, err error) {
			if refusedByAPI(err) {
				d.refuse(gvr, w, err)
				return
			}
			cache.DefaultWatchErrorHandler(ctx, reflector, err)
		})
}

// refusedByAPI reports list errors a retry will not fix soon.
func refusedByAPI(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) ||
		apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err)
}

// cappedHandler translates notifications for admitted objects only (see
// admission) and drops updates that change nothing but the resource
// version, such as Lease renewals seen through metadata. A stub is never
// translated; an object admitted after its stub reads as present, not as
// a change, because the stub holds no previous state to compare.
func cappedHandler(
	ctx context.Context, w *dynamicWatch, submit Submit,
	now func() time.Time,
) cache.ResourceEventHandler {
	send := func(observations []inventory.Observation) {
		if len(observations) > 0 {
			submit(ctx, observations...)
		}
	}
	t, a := w.translator, w.admission
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, initialList bool) {
			if !isCappedStub(obj) && a.admit(obj) {
				send(t.Added(obj, initialList, now()))
			}
		},
		UpdateFunc: func(old, new any) {
			if isCappedStub(new) || versionOnlyChange(old, new) ||
				!a.admit(new) {
				return
			}
			if isCappedStub(old) {
				send(t.Added(new, true, now()))
				return
			}
			send(t.Updated(old, new, now()))
		},
		DeleteFunc: func(obj any) {
			if a.forget(obj) {
				send(t.Deleted(obj, now()))
			}
		},
	}
}

// versionOnlyChange reports an update whose object differs only in its
// resource version and managed fields. Resyncs, which repeat the same
// version, are not dropped.
func versionOnlyChange(old, new any) bool {
	before, err1 := meta.Accessor(old)
	after, err2 := meta.Accessor(new)
	if err1 != nil || err2 != nil ||
		before.GetResourceVersion() == after.GetResourceVersion() {
		return false
	}
	switch a := new.(type) {
	case *metav1.PartialObjectMetadata:
		b, ok := old.(*metav1.PartialObjectMetadata)
		return ok && reflect.DeepEqual(
			comparableMeta(b.ObjectMeta), comparableMeta(a.ObjectMeta))
	case *unstructured.Unstructured:
		b, ok := old.(*unstructured.Unstructured)
		return ok && sameIgnoringVersion(b.Object, a.Object)
	}
	return false
}

func comparableMeta(m metav1.ObjectMeta) metav1.ObjectMeta {
	m.ResourceVersion = ""
	m.ManagedFields = nil
	return m
}

func sameIgnoringVersion(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if key == "metadata" {
			continue
		}
		if !reflect.DeepEqual(value, b[key]) {
			return false
		}
	}
	am, _ := a["metadata"].(map[string]any)
	bm, _ := b["metadata"].(map[string]any)
	return sameMetadata(am, bm)
}

func sameMetadata(a, b map[string]any) bool {
	skip := func(key string) bool {
		return key == "resourceVersion" || key == "managedFields"
	}
	for key, value := range a {
		if !skip(key) && !reflect.DeepEqual(value, b[key]) {
			return false
		}
	}
	for key := range b {
		if _, ok := a[key]; !ok && !skip(key) {
			return false
		}
	}
	return true
}

// rediscoveryHandler requests discovery when a CRD or APIService is
// added after the initial list, deleted, or changes its generation or
// conditions (a CRD becoming established, an aggregated API becoming
// available).
func rediscoveryHandler(request func()) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(_ any, initialList bool) {
			if !initialList {
				request()
			}
		},
		UpdateFunc: func(old, new any) {
			if discoveryRelevant(old, new) {
				request()
			}
		},
		DeleteFunc: func(any) { request() },
	}
}

func discoveryRelevant(old, new any) bool {
	before, ok1 := old.(*unstructured.Unstructured)
	after, ok2 := new.(*unstructured.Unstructured)
	if !ok1 || !ok2 {
		return false
	}
	if before.GetGeneration() != after.GetGeneration() {
		return true
	}
	a, _, _ := unstructured.NestedSlice(before.Object, "status", "conditions")
	b, _, _ := unstructured.NestedSlice(after.Object, "status", "conditions")
	return !reflect.DeepEqual(conditionStatuses(a), conditionStatuses(b))
}

func conditionStatuses(raw []any) map[string]string {
	out := map[string]string{}
	for _, c := range parseConditions(raw) {
		out[c.Type] = c.Status
	}
	return out
}
