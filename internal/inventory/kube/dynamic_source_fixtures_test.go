package kube

import (
	"context"
	"errors"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
)

// lockedDiscovery is a fake discovery client whose served resources can
// change while a source runs.
type lockedDiscovery struct {
	*fakediscovery.FakeDiscovery
	mu    sync.Mutex
	lists []*metav1.APIResourceList
	err   error
}

func newLockedDiscovery(lists ...*metav1.APIResourceList) *lockedDiscovery {
	return &lockedDiscovery{
		FakeDiscovery: &fakediscovery.FakeDiscovery{
			Fake: &clienttesting.Fake{},
		},
		lists: lists,
	}
}

func (f *lockedDiscovery) set(lists ...*metav1.APIResourceList) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = lists
}

func (f *lockedDiscovery) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// ServerGroupsAndResourcesWithContext serves the current lists; each
// group's first listed version is its preferred version.
func (f *lockedDiscovery) ServerGroupsAndResourcesWithContext(
	context.Context,
) ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var groups []*metav1.APIGroup
	byName := map[string]*metav1.APIGroup{}
	for _, list := range f.lists {
		gv, _ := schema.ParseGroupVersion(list.GroupVersion)
		version := metav1.GroupVersionForDiscovery{
			GroupVersion: list.GroupVersion, Version: gv.Version,
		}
		group := byName[gv.Group]
		if group == nil {
			group = &metav1.APIGroup{Name: gv.Group,
				PreferredVersion: version}
			byName[gv.Group] = group
			groups = append(groups, group)
		}
		group.Versions = append(group.Versions, version)
	}
	return groups, f.lists, f.err
}

// apiList builds one served group version.
func apiList(
	groupVersion string, resources ...[]metav1.APIResource,
) *metav1.APIResourceList {
	list := &metav1.APIResourceList{GroupVersion: groupVersion}
	for _, r := range resources {
		list.APIResources = append(list.APIResources, r...)
	}
	return list
}

// servedResource is a list-and-watch resource, with its status
// subresource when status is set.
func servedResource(name, kind string, status bool) []metav1.APIResource {
	out := []metav1.APIResource{{
		Name: name, Kind: kind,
		Verbs: metav1.Verbs{"get", "list", "watch"},
	}}
	if status {
		out = append(out, metav1.APIResource{
			Name: name + "/status", Kind: kind,
			Verbs: metav1.Verbs{"get", "update"},
		})
	}
	return out
}

// anchorLists serves the CRD and APIService APIs.
func anchorLists() []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		apiList("apiextensions.k8s.io/v1", servedResource(
			"customresourcedefinitions", "CustomResourceDefinition", true)),
		apiList("apiregistration.k8s.io/v1", servedResource(
			"apiservices", "APIService", true)),
	}
}

func widgetList() *metav1.APIResourceList {
	return apiList("example.com/v1",
		servedResource("widgets", "Widget", true))
}

func forbidden(resource string) error {
	return apierrors.NewForbidden(
		schema.GroupResource{Group: "example.com", Resource: resource},
		"", errors.New("denied"))
}
