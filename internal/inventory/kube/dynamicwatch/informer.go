package dynamicwatch

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"
)

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

// NewMetadataInformer constructs one metadata-only informer across all
// namespaces with the given transform. Its objects are
// *metav1.PartialObjectMetadata.
func NewMetadataInformer(
	client metadata.Interface,
	resync time.Duration,
	gvr schema.GroupVersionResource,
	transform cache.TransformFunc,
) (cache.SharedIndexInformer, error) {
	if client == nil {
		return nil, fmt.Errorf("metadata watcher has no client")
	}
	informer := metadatainformer.NewFilteredMetadataInformer(
		client, gvr, metav1.NamespaceAll, resync, cache.Indexers{}, nil,
	).Informer()
	if transform != nil {
		if err := informer.SetTransform(transform); err != nil {
			return nil, fmt.Errorf(
				"set %s cache transform: %w", gvr, err,
			)
		}
	}
	return informer, nil
}
