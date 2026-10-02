package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func catalogResource(e CoverageEntry) Resource {
	return Resource{Group: e.Group, Name: e.Resource}
}

// catalogList is the discovery answer for one catalog entry.
func catalogList(e CoverageEntry) *metav1.APIResourceList {
	gv := "v1"
	if e.Group != "" {
		gv = e.Group + "/v1"
	}
	resources := []metav1.APIResource{{Name: e.Resource, Kind: e.Kind,
		Verbs: metav1.Verbs{"list", "watch", "get"}}}
	if e.HasStatus {
		resources = append(resources, metav1.APIResource{
			Name: e.Resource + "/status", Kind: e.Kind,
			Verbs: metav1.Verbs{"get"}})
	}
	return &metav1.APIResourceList{GroupVersion: gv,
		APIResources: resources}
}

func TestCoverageCatalogEntriesAreUnique(t *testing.T) {
	seen := map[Resource]bool{}
	for _, e := range CoverageCatalog() {
		r := catalogResource(e)
		require.False(t, seen[r], "duplicate %+v", r)
		seen[r] = true
	}
}

func TestCoverageCatalogModesMatchWatchPlan(t *testing.T) {
	typed := typedResources()
	modes := map[Resource]WatchMode{}
	for _, r := range registrations() {
		modes[r.resource] = r.mode()
	}
	for _, e := range CoverageCatalog() {
		r := catalogResource(e)
		t.Run(e.Group+"/"+e.Resource, func(t *testing.T) {
			switch e.Mode {
			case WatchFull, WatchHashed:
				require.True(t, typed[r], "not a typed kind")
				if r != eventsResource {
					assert.Equal(t, e.Mode, modes[r])
				}
			case CoverageModeExcluded:
				assert.Empty(t, planList(catalogList(e), typed,
					map[Resource]bool{}))
			default:
				planned := planList(catalogList(e), typed,
					map[Resource]bool{})
				require.Len(t, planned, 1, "kind is not watched")
				assert.Equal(t, e.Mode, planned[0].mode)
				assert.Equal(t, e.Kind, planned[0].kind)
			}
		})
	}
}

func TestCoverageCatalogCoversEveryPlanDecision(t *testing.T) {
	listed := map[Resource]bool{}
	dynamic := 0
	for _, e := range CoverageCatalog() {
		listed[catalogResource(e)] = true
		if e.Mode != WatchFull && e.Mode != WatchHashed &&
			e.Mode != CoverageModeExcluded {
			dynamic++
		}
	}
	for _, r := range registrations() {
		assert.True(t, listed[r.resource], "typed %+v missing", r.resource)
	}
	for r := range metadataResources {
		assert.True(t, listed[r], "metadata %+v missing", r)
	}
	for r := range excludedResources {
		assert.True(t, listed[r], "excluded %+v missing", r)
	}
	assert.LessOrEqual(t, dynamic, DefaultResourceBudget,
		"catalog kinds must fit the default watch budget")
}
