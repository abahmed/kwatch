package kube

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
)

// discoverResources lists every served resource type that supports list
// and watch: built-in groups, CRDs and aggregated APIs alike. Each group
// and resource is taken once, from the group's preferred version when it
// serves it, otherwise from the first version that does. The result is
// complete only when every group version answered; callers must not
// retire watches after an incomplete discovery.
func discoverResources(
	ctx context.Context, client discovery.DiscoveryInterfaceWithContext,
) ([]plannedResource, bool) {
	if client == nil {
		return nil, false
	}
	groups, lists, err := client.ServerGroupsAndResourcesWithContext(ctx)
	complete := err == nil
	if err != nil {
		klog.V(2).InfoS("resource discovery incomplete",
			"component", "inventory", "operation", "discover",
			"partial", discovery.IsGroupDiscoveryFailedError(err),
			"error", err)
		if len(lists) == 0 {
			return nil, false
		}
	}
	byVersion := make(map[string]*metav1.APIResourceList, len(lists))
	for _, list := range lists {
		if list != nil {
			byVersion[list.GroupVersion] = list
		}
	}
	typed := typedResources()
	taken := map[Resource]bool{}
	var out []plannedResource
	for _, group := range groups {
		if group == nil {
			continue
		}
		for _, gv := range versionOrder(group) {
			list := byVersion[gv]
			if list == nil {
				continue
			}
			out = append(out, planList(list, typed, taken)...)
		}
	}
	return out, complete
}

// versionOrder is the preferred version first, then the others in the
// order the server lists them.
func versionOrder(group *metav1.APIGroup) []string {
	out := []string{group.PreferredVersion.GroupVersion}
	for _, v := range group.Versions {
		if v.GroupVersion != group.PreferredVersion.GroupVersion {
			out = append(out, v.GroupVersion)
		}
	}
	return out
}

// planList plans the list-and-watch resources of one group version not
// already taken from another version.
func planList(
	list *metav1.APIResourceList, typed, taken map[Resource]bool,
) []plannedResource {
	gv, err := schema.ParseGroupVersion(list.GroupVersion)
	if err != nil {
		return nil
	}
	statuses := map[string]bool{}
	for _, r := range list.APIResources {
		if parent, ok := strings.CutSuffix(r.Name, "/status"); ok {
			statuses[parent] = true
		}
	}
	var out []plannedResource
	for _, r := range list.APIResources {
		key := Resource{Group: gv.Group, Name: r.Name}
		if strings.Contains(r.Name, "/") || taken[key] ||
			!listAndWatch(r.Verbs) {
			continue
		}
		taken[key] = true
		p, ok := planResource(gv.WithResource(r.Name), r.Kind,
			statuses[r.Name], typed)
		if ok {
			out = append(out, p)
		}
	}
	return out
}

func listAndWatch(verbs metav1.Verbs) bool {
	var list, watch bool
	for _, verb := range verbs {
		list = list || verb == "list"
		watch = watch || verb == "watch"
	}
	return list && watch
}
