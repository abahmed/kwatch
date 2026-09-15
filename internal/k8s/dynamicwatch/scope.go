package dynamicwatch

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	if client == nil {
		return true
	}
	resources, err := client.ServerResourcesForGroupVersion(
		gvr.GroupVersion().String(),
	)
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
