package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/handler"
)

// wireEvents wires the pod-event informers. It returns the additional factory
// instances (one per monitored namespace) that must be started.
func (c *Controller) wireEvents(
	client kubernetes.Interface,
	resync time.Duration,
	scope namespaceScope,
) []informers.SharedInformerFactory {
	var eventFactories []informers.SharedInformerFactory
	if !scope.all && len(scope.namespaces) == 0 {
		return eventFactories
	}
	warningFactories := wireWarningEvents(c.handler, client, resync, scope)
	eventFactories = append(eventFactories, warningFactories...)
	for _, factory := range warningFactories {
		c.eventsSynced = append(c.eventsSynced, factory.Core().V1().Events().Informer().HasSynced)
	}
	if scope.all || len(scope.namespaces) == 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "involvedObject.kind=Pod"
				if scope.all {
					o.FieldSelector += "," + informerExcludedNamespaces(scope.forbidden)
				}
			}),
		}
		if len(scope.namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(scope.namespaces[0]))
		}
		opts = append(opts, informerMemoryOptions()...)
		ef := informers.NewSharedInformerFactoryWithOptions(
			client,
			resync,
			opts...)
		eventFactories = append(eventFactories, ef)
		eventInformer := ef.Core().V1().Events().Informer()
		utilruntime.Must(eventInformer.AddIndexers(cache.Indexers{
			"byPod": func(obj interface{}) ([]string, error) {
				if ev, ok := obj.(*corev1.Event); ok {
					return []string{
						eventsByPodKey(
							ev.InvolvedObject.Namespace,
							ev.InvolvedObject.Name,
						),
					}, nil
				}
				return nil, nil
			},
		}))
		c.eventLister = ef.Core().V1().Events().Lister()
		c.eventIndexers = append(c.eventIndexers, eventInformer.GetIndexer())
		c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
	} else {
		listers := make([]corev1lister.EventLister, 0, len(scope.namespaces))
		for _, ns := range scope.namespaces {
			ns := ns
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "involvedObject.kind=Pod"
				}),
				informers.WithNamespace(ns),
			}
			opts = append(opts, informerMemoryOptions()...)
			ef := informers.NewSharedInformerFactoryWithOptions(
				client,
				resync,
				opts...)
			eventFactories = append(eventFactories, ef)
			eventInformer := ef.Core().V1().Events().Informer()
			utilruntime.Must(eventInformer.AddIndexers(cache.Indexers{
				"byPod": func(obj interface{}) ([]string, error) {
					if ev, ok := obj.(*corev1.Event); ok {
						return []string{
							eventsByPodKey(
								ev.InvolvedObject.Namespace,
								ev.InvolvedObject.Name,
							),
						}, nil
					}
					return nil, nil
				},
			}))
			listers = append(listers, ef.Core().V1().Events().Lister())
			c.eventIndexers = append(
				c.eventIndexers,
				eventInformer.GetIndexer(),
			)
			c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
		}
		c.eventLister = &multiEventLister{listers: listers}
	}
	return eventFactories
}

// wireWarningEvents adds a second, deliberately narrow event stream for
// resource-level failures. Pod Events remain on the indexed informer above so
// their richer log/container context is preserved.
func wireWarningEvents(
	h handler.Handler,
	client kubernetes.Interface,
	resync time.Duration,
	scope namespaceScope,
) []informers.SharedInformerFactory {
	var factories []informers.SharedInformerFactory
	add := func(opts ...informers.SharedInformerOption) {
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		informer := factory.Core().V1().Events().Informer()
		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if ev, ok := obj.(*corev1.Event); ok {
					h.ProcessWarningEvent(ev)
				}
			},
			UpdateFunc: func(_, obj interface{}) {
				if ev, ok := obj.(*corev1.Event); ok {
					h.ProcessWarningEvent(ev)
				}
			},
		})
		factories = append(factories, factory)
	}
	if scope.all || len(scope.namespaces) == 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "type=Warning"
				if scope.all {
					o.FieldSelector += "," + informerExcludedNamespaces(scope.forbidden)
				}
			}),
		}
		if len(scope.namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(scope.namespaces[0]))
		}
		add(opts...)
		return factories
	}
	for _, namespace := range scope.namespaces {
		namespace := namespace
		add(
			informers.WithNamespace(namespace),
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "type=Warning"
			}),
		)
	}
	return factories
}

// eventsByPodIndex is the informer index that maps "namespace/pod" to the
// events involving that pod.
const eventsByPodIndex = "byPod"

func eventsByPodKey(namespace, pod string) string {
	return namespace + "/" + pod
}

// eventsByPod resolves a pod's events straight from the informer index. With
// several namespaced informers each holds a disjoint slice of the cluster, so
// asking all of them is still one indexed lookup each rather than a scan.
func (c *Controller) eventsByPod(
	namespace, pod string,
) ([]*corev1.Event, error) {
	var out []*corev1.Event
	for _, ix := range c.eventIndexers {
		objs, err := ix.ByIndex(
			eventsByPodIndex,
			eventsByPodKey(namespace, pod),
		)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			if ev, ok := o.(*corev1.Event); ok {
				out = append(out, ev)
			}
		}
	}
	return out, nil
}

// wireClusterAutoscaler wires the cluster-autoscaler event informer and returns
// its dedicated factory.
func wireClusterAutoscaler(
	h handler.Handler,
	client kubernetes.Interface,
	resync time.Duration,
) informers.SharedInformerFactory {
	caOpts := []informers.SharedInformerOption{
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.FieldSelector = "source=cluster-autoscaler"
		}),
		informers.WithNamespace("kube-system"),
	}
	caFactory := informers.NewSharedInformerFactoryWithOptions(
		client,
		resync,
		caOpts...)
	caEventInformer := caFactory.Core().V1().Events().Informer()
	utilruntime.Must(caEventInformer.AddIndexers(cache.Indexers{
		"byReason": func(obj interface{}) ([]string, error) {
			if ev, ok := obj.(*corev1.Event); ok {
				return []string{ev.Reason}, nil
			}
			return nil, nil
		},
	}))
	caEventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if ev, ok := obj.(*corev1.Event); ok {
				h.ProcessClusterAutoscalerEvent(ev)
			}
		},
		UpdateFunc: func(_, obj interface{}) {
			if ev, ok := obj.(*corev1.Event); ok {
				h.ProcessClusterAutoscalerEvent(ev)
			}
		},
	})
	return caFactory
}
