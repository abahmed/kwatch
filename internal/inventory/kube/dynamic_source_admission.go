package kube

import (
	"sync"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// objectBudget counts the objects every dynamic watch reports, against
// the global DefaultObjectBudget.
type objectBudget struct {
	limit int64
	used  atomic.Int64
}

func (b *objectBudget) take() bool {
	if b.used.Add(1) > b.limit {
		b.used.Add(-1)
		return false
	}
	return true
}

func (b *objectBudget) release(n int) { b.used.Add(int64(-n)) }

// objectAdmission decides which objects of one watched type produce
// observations: at most perKind, within the shared budget. An object
// refused once is admitted by a later update when room has freed.
//
// It remembers the last admitted version of every object, so a watch
// restarted after a refusal can report the objects that disappeared
// meanwhile (dropMissing), and the objects it could not admit, so the
// type is not verifiable while any is left out (capped).
type objectAdmission struct {
	gvr     schema.GroupVersionResource
	perKind int
	budget  *objectBudget
	mu      sync.Mutex
	objects map[string]any
	left    map[string]bool
	warned  bool
	refused atomic.Int64
}

func newAdmission(
	gvr schema.GroupVersionResource, perKind int, budget *objectBudget,
) *objectAdmission {
	return &objectAdmission{
		gvr: gvr, perKind: perKind, budget: budget,
		objects: map[string]any{}, left: map[string]bool{},
	}
}

// admit reports whether obj may produce observations, admitting it when
// there is room.
func (a *objectAdmission) admit(obj any) bool {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.objects[key]; ok {
		a.objects[key] = obj
		return true
	}
	if len(a.objects) < a.perKind && a.budget.take() {
		a.objects[key] = obj
		delete(a.left, key)
		return true
	}
	a.left[key] = true
	a.refused.Add(1)
	if !a.warned {
		a.warned = true
		klog.InfoS("object cap reached; further objects are not observed",
			"component", "inventory", "operation", "watch",
			"resource", a.gvr.String(), "perKind", a.perKind,
			"budget", a.budget.limit)
	}
	return false
}

// forget releases a deleted object and reports whether it was admitted.
func (a *objectAdmission) forget(obj any) bool {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.left, key)
	if _, ok := a.objects[key]; !ok {
		return false
	}
	delete(a.objects, key)
	a.budget.release(1)
	return true
}

// admitted reports whether obj was admitted, without admitting it.
func (a *objectAdmission) admitted(obj any) bool {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.objects[key]
	return ok
}

// capped reports whether some current object was left out by a cap.
func (a *objectAdmission) capped() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.left) > 0
}

// dropMissing forgets every admitted object that present does not
// report and returns their last admitted versions.
func (a *objectAdmission) dropMissing(present func(key string) bool) []any {
	a.mu.Lock()
	defer a.mu.Unlock()
	var missing []any
	for key, obj := range a.objects {
		if !present(key) {
			missing = append(missing, obj)
			delete(a.objects, key)
		}
	}
	for key := range a.left {
		if !present(key) {
			delete(a.left, key)
		}
	}
	a.budget.release(len(missing))
	return missing
}

// releaseAll returns every admitted object to the shared budget when the
// watch is retired or stopped.
func (a *objectAdmission) releaseAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.budget.release(len(a.objects))
	a.objects = map[string]any{}
	a.left = map[string]bool{}
}
