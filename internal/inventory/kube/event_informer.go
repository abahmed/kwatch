package kube

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// warningEventSelector asks the API server for Warning events only.
// EventNote ignores every other type, so caching Normal events (most of
// the event volume in a busy cluster) would only cost memory.
var warningEventSelector = fields.OneTermEqualSelector(
	"type", corev1.EventTypeWarning).String()

// warningEventInformer registers the cluster-wide Event informer on
// factory with the Warning field selector. The factory still owns its
// start, transform and shutdown.
func warningEventInformer(
	factory informers.SharedInformerFactory,
) cache.SharedIndexInformer {
	return factory.InformerFor(&corev1.Event{},
		func(c kubernetes.Interface, resync time.Duration,
		) cache.SharedIndexInformer {
			return coreinformers.NewFilteredEventInformer(
				c, metav1.NamespaceAll, resync,
				cache.Indexers{
					cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
				},
				func(options *metav1.ListOptions) {
					options.FieldSelector = warningEventSelector
				})
		})
}
