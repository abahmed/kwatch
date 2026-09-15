package dynamicwatch

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// Factory is the informer factory type owned by this package. It is exposed
// only so specialized watchers can retain their lifecycle policies without
// reimplementing informer construction.
type Factory = dynamicinformer.DynamicSharedInformerFactory

// NewInformer constructs one namespaced dynamic informer with the shared
// transform policy. Callers retain ownership of event handlers and lifecycle
// because some resources, such as KwatchConfig, have special restart rules.
func NewInformer(
	client dynamic.Interface,
	resync time.Duration,
	namespace string,
	gvr schema.GroupVersionResource,
	transform cache.TransformFunc,
) (dynamicinformer.DynamicSharedInformerFactory,
	cache.SharedIndexInformer, error) {
	if client == nil {
		return nil, nil, fmt.Errorf("dynamic watcher has no client")
	}
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		client, resync, namespace, nil,
	)
	informer := factory.ForResource(gvr).Informer()
	if transform != nil {
		if err := informer.SetTransform(transform); err != nil {
			return nil, nil, fmt.Errorf(
				"set %s cache transform: %w", gvr, err,
			)
		}
	}
	return factory, informer, nil
}
