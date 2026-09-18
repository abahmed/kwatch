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
	client discovery.DiscoveryInterface,
	gvr schema.GroupVersionResource,
) bool {
	return ResourceAvailableContext(context.Background(), client, gvr)
}

// ResourceAvailableContext uses client-go's context-aware discovery path when
// the application client provides it. Legacy discovery implementations are
// kept for tests and compatibility; they cannot be canceled by this package.
func ResourceAvailableContext(
	ctx context.Context,
	client discovery.DiscoveryInterface,
	gvr schema.GroupVersionResource,
) bool {
	if client == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if contextual, ok := client.(discovery.DiscoveryInterfaceWithContext); ok {
		resources, err := contextual.ServerResourcesForGroupVersionWithContext(
			ctx, gvr.GroupVersion().String(),
		)
		return resourceIsAvailable(resources, err, gvr)
	}
	resources, err := client.ServerResourcesForGroupVersion(
		gvr.GroupVersion().String(),
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
