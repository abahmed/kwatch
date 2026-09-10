package controller

import (
	"strings"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/handler"
)

// enqueueServiceDependents rechecks the objects whose detectors read this
// Service, because a Service appearing or disappearing can open or resolve an
// Ingress or admission-webhook incident without the referencing object
// changing at all.
//
// It re-enqueues only the objects that actually name this Service. Every
// Service event used to re-enqueue every Ingress and every webhook
// configuration in the cluster; on a busy cluster that is thousands of
// redundant syncs a minute, and it hid the cheap targeted lookup the graph
// already supports. The periodic informer resync is what covers a graph that
// is briefly stale.
func (c *Controller) enqueueServiceDependents(obj interface{}) {
	service, ok := obj.(*corev1.Service)
	if !ok {
		// A tombstone or unexpected type: fall back to the broad recheck
		// rather than silently skipping dependents.
		c.enqueueAllServiceDependents()
		return
	}
	if c.ingress.startWorkers {
		c.enqueueIngressesForService(service)
	}
	c.enqueueWebhooksForService(service)
}

// enqueueIngressesForService re-enqueues the Ingresses that name this
// Service.
//
// The live Ingress spec is the authority, not the graph: an Ingress backend
// is always in the Ingress's own namespace, so scanning that one namespace
// and matching by name finds every dependent and cannot miss one because the
// graph was briefly stale -- which is why the previous code followed the
// graph with a cluster-wide re-enqueue of every Ingress. The graph's reverse
// index is still consulted, so an edge kind the spec scan does not model is
// not lost.
func (c *Controller) enqueueIngressesForService(service *corev1.Service) {
	enqueued := make(map[string]bool)
	if c.ingressLister != nil {
		items, err := c.ingressLister.Ingresses(service.Namespace).List(
			labels.Everything(),
		)
		if err == nil {
			for _, ing := range items {
				if !ingressReferencesService(ing, service.Name) {
					continue
				}
				enqueued[ing.Namespace+"/"+ing.Name] = true
				c.ingress.enqueue(ing)
			}
		}
	}
	if c.graph == nil {
		return
	}
	for _, key := range c.graph.DependentsByType(
		"service", service.Namespace, service.Name, "ingress",
	) {
		ref := strings.TrimPrefix(key, "ingress/")
		if enqueued[ref] {
			continue
		}
		c.ingress.enqueue(ref)
	}
}

// enqueueWebhooksForService re-enqueues only the webhook configurations whose
// client config names this Service.
func (c *Controller) enqueueWebhooksForService(service *corev1.Service) {
	c.enqueueWebhooksForServiceKey(service.Namespace, service.Name)
}

func (c *Controller) enqueueWebhooksForServiceKey(namespace, name string) {
	if c.mwc != nil && c.mwc.startWorkers && c.mwcLister != nil {
		if items, err := c.mwcLister.List(labels.Everything()); err == nil {
			for _, item := range items {
				if webhookRefsService(
					handler.MutatingWebhookServices(item),
					namespace,
					name,
				) {
					c.mwc.enqueue(item)
				}
			}
		}
	}
	if c.vwc != nil && c.vwc.startWorkers && c.vwcLister != nil {
		if items, err := c.vwcLister.List(labels.Everything()); err == nil {
			for _, item := range items {
				if webhookRefsService(
					handler.ValidatingWebhookServices(item),
					namespace,
					name,
				) {
					c.vwc.enqueue(item)
				}
			}
		}
	}
}

func webhookRefsService(
	refs []*admissionregistrationv1.ServiceReference,
	namespace, name string,
) bool {
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		if ref.Namespace == namespace && ref.Name == name {
			return true
		}
	}
	return false
}

func ingressReferencesService(
	ing *networkingv1.Ingress,
	name string,
) bool {
	if ing.Spec.DefaultBackend != nil &&
		ing.Spec.DefaultBackend.Service != nil &&
		ing.Spec.DefaultBackend.Service.Name == name {
		return true
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil &&
				path.Backend.Service.Name == name {
				return true
			}
		}
	}
	return false
}

// enqueueAllServiceDependents is the conservative path used when the changed
// object could not be read.
func (c *Controller) enqueueAllServiceDependents() {
	if c.ingress.startWorkers {
		c.enqueueAllIngresses()
	}
	if c.mwc.startWorkers && c.mwcLister != nil {
		if items, err := c.mwcLister.List(labels.Everything()); err == nil {
			for _, item := range items {
				c.mwc.enqueue(item)
			}
		}
	}
	if c.vwc.startWorkers && c.vwcLister != nil {
		if items, err := c.vwcLister.List(labels.Everything()); err == nil {
			for _, item := range items {
				c.vwc.enqueue(item)
			}
		}
	}
}

func (c *Controller) enqueueAllIngresses() {
	if c.ingressLister == nil {
		return
	}
	items, err := c.ingressLister.Ingresses(metav1.NamespaceAll).List(
		labels.Everything(),
	)
	if err == nil {
		for _, item := range items {
			c.ingress.enqueue(item)
		}
	}
}
