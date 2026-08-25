package controller

// Multi-lister adapters.
//
// When kwatch watches a set of namespaces (or the whole cluster plus a
// cluster-scoped factory), each resource gets one lister per informer
// factory. The multiXLister types below fan a single typed lister API out
// over all of them:
//
//   - List merges every sub-lister's results in factory order; the first
//     error aborts.
//   - Get returns the first object found across factories, or NotFound if
//     none has it — matching the single-factory lister semantics callers
//     already know.

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ---------------------------------------------------------------------------
// Generic core
// ---------------------------------------------------------------------------

type listerOf[Item any] interface {
	List(labels.Selector) ([]Item, error)
}

type getterOf[Item any] interface {
	Get(string) (Item, error)
}

// listAll merges List results across factories in order; first error aborts.
func listAll[Item any, L listerOf[Item]](selector labels.Selector, ls []L) ([]Item, error) {
	var all []Item
	for _, l := range ls {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

// nsAll collects the namespace lister for ns from every factory.
func nsAll[L any, NsL any](ls []L, ns string, fn func(L, string) NsL) []NsL {
	out := make([]NsL, 0, len(ls))
	for _, l := range ls {
		out = append(out, fn(l, ns))
	}
	return out
}

// getFirst returns the first item found across namespace listers, or a
// NotFound error carrying gr if none has it.
func getFirst[Item any, G getterOf[Item]](name string, gr schema.GroupResource, gs []G) (Item, error) {
	for _, g := range gs {
		item, err := g.Get(name)
		if err == nil {
			return item, nil
		}
	}
	var zero Item
	return zero, apierrors.NewNotFound(gr, name)
}

// multiNamespace is the shared implementation of every per-resource
// namespace lister below. NsL is the concrete typed namespace lister,
// which always supports both List and Get.
type multiNamespace[Item any, NsL interface {
	listerOf[Item]
	getterOf[Item]
}] struct {
	listers []NsL
	gr      schema.GroupResource
}

func (m *multiNamespace[Item, _]) List(selector labels.Selector) ([]Item, error) {
	return listAll(selector, m.listers)
}

func (m *multiNamespace[Item, _]) Get(name string) (Item, error) {
	return getFirst(name, m.gr, m.listers)
}

// ---------------------------------------------------------------------------
