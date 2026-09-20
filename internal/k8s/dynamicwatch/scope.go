package dynamicwatch

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// Namespaces returns the informer namespaces for a resource. Cluster-scoped
// resources and all-namespace watches use the empty namespace.
func Namespaces(
	namespaced bool,
	namespaces []string,
	watchAll bool,
) []string {
	if !namespaced || watchAll {
		return []string{""}
	}
	return append([]string(nil), namespaces...)
}

// ResourceAvailable reports whether an optional resource should be watched.
// Permanent discovery failures are skipped; transient failures are allowed to
// reach the informer so its normal retry behavior can recover.
func ResourceAvailable(
	client discovery.DiscoveryInterfaceWithContext,
	gvr schema.GroupVersionResource,
) bool {
	return ResourceAvailableContext(context.Background(), client, gvr)
}

// ResourceAvailableContext requires client-go's context-aware discovery path so
// discovery cannot outlive a watcher generation.
func ResourceAvailableContext(
	ctx context.Context,
	client discovery.DiscoveryInterfaceWithContext,
	gvr schema.GroupVersionResource,
) bool {
	if client == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resources, err := client.ServerResourcesForGroupVersionWithContext(
		ctx, gvr.GroupVersion().String(),
	)
	return resourceIsAvailable(resources, err, gvr)
}

func resourceIsAvailable(
	resources *metav1.APIResourceList,
	err error,
	gvr schema.GroupVersionResource,
) bool {
	if err != nil {
		return !apierrors.IsNotFound(err) &&
			!apierrors.IsForbidden(err) &&
			!apierrors.IsUnauthorized(err) &&
			!apierrors.IsMethodNotSupported(err)
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return true
		}
	}
	return false
}
