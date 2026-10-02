package kube

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func plannedModes(resources []plannedResource) map[string]WatchMode {
	out := map[string]WatchMode{}
	for _, r := range resources {
		out[r.gvr.Group+"/"+r.gvr.Version+"/"+r.gvr.Resource] = r.mode
	}
	return out
}

func TestDiscoverResourcesPlansEveryListWatchType(t *testing.T) {
	disco := newLockedDiscovery(append(anchorLists(),
		apiList("v1",
			servedResource("pods", "Pod", true),
			servedResource("configmaps", "ConfigMap", false),
			servedResource("endpoints", "Endpoints", false),
			servedResource("replicationcontrollers",
				"ReplicationController", true),
			[]metav1.APIResource{{Name: "componentstatuses",
				Kind: "ComponentStatus", Verbs: metav1.Verbs{"get", "list"}}}),
		apiList("coordination.k8s.io/v1",
			servedResource("leases", "Lease", false)),
		apiList("apps/v1",
			servedResource("controllerrevisions", "ControllerRevision", false),
			servedResource("deployments", "Deployment", true)),
		apiList("rbac.authorization.k8s.io/v1",
			servedResource("roles", "Role", false),
			servedResource("clusterrolebindings", "ClusterRoleBinding", false)),
		apiList("scheduling.k8s.io/v1",
			servedResource("priorityclasses", "PriorityClass", false)),
		apiList("flowcontrol.apiserver.k8s.io/v1",
			servedResource("flowschemas", "FlowSchema", true)),
		apiList("events.k8s.io/v1", servedResource("events", "Event", false)),
		apiList("example.com/v1",
			servedResource("widgets", "Widget", true),
			servedResource("plains", "Plain", false)),
		// An aggregated API: served by an extension apiserver, no CRD.
		apiList("aggregated.example.io/v1",
			servedResource("things", "Thing", false)),
		apiList("metrics.k8s.io/v1beta1", []metav1.APIResource{{
			Name: "pods", Kind: "PodMetrics",
			Verbs: metav1.Verbs{"get", "list"}}}),
	)...)

	resources, complete := discoverResources(context.Background(), disco)

	assert.True(t, complete)
	assert.Equal(t, map[string]WatchMode{
		"apiextensions.k8s.io/v1/customresourcedefinitions": WatchStatus,
		"apiregistration.k8s.io/v1/apiservices":             WatchStatus,
		"/v1/replicationcontrollers":                        WatchStatus,
		"coordination.k8s.io/v1/leases":                     WatchMetadata,
		"apps/v1/controllerrevisions":                       WatchMetadata,
		"rbac.authorization.k8s.io/v1/roles":                WatchMetadata,
		"rbac.authorization.k8s.io/v1/clusterrolebindings":  WatchMetadata,
		"scheduling.k8s.io/v1/priorityclasses":              WatchMetadata,
		"flowcontrol.apiserver.k8s.io/v1/flowschemas":       WatchStatus,
		"example.com/v1/widgets":                            WatchStatus,
		"example.com/v1/plains":                             WatchStatus,
		"aggregated.example.io/v1/things":                   WatchStatus,
	}, plannedModes(resources))
}

func TestDiscoverResourcesPrefersServedVersion(t *testing.T) {
	policies := func(version string, names ...string) *metav1.APIResourceList {
		list := apiList("admissionregistration.k8s.io/" + version)
		for _, name := range names {
			list.APIResources = append(list.APIResources,
				servedResource(name, "X", true)...)
		}
		return list
	}
	tt := []struct {
		name   string
		served []*metav1.APIResourceList
		want   map[string]WatchMode
	}{
		{
			name: "beta_only",
			served: []*metav1.APIResourceList{policies("v1beta1",
				"mutatingadmissionpolicies")},
			want: map[string]WatchMode{
				"admissionregistration.k8s.io/v1beta1/" +
					"mutatingadmissionpolicies": WatchStatus,
			},
		},
		{
			name: "preferred_version_wins",
			served: []*metav1.APIResourceList{
				policies("v1", "mutatingadmissionpolicies"),
				policies("v1beta1", "mutatingadmissionpolicies",
					"mutatingadmissionpolicybindings"),
			},
			want: map[string]WatchMode{
				"admissionregistration.k8s.io/v1/" +
					"mutatingadmissionpolicies": WatchStatus,
				"admissionregistration.k8s.io/v1beta1/" +
					"mutatingadmissionpolicybindings": WatchStatus,
			},
		},
		{name: "not_served", want: map[string]WatchMode{}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := discoverResources(context.Background(),
				newLockedDiscovery(tc.served...))
			assert.Equal(t, tc.want, plannedModes(got))
		})
	}
}

func TestDiscoverResourcesReportsFailure(t *testing.T) {
	disco := newLockedDiscovery()
	disco.fail(errors.New("unavailable"))
	got, complete := discoverResources(context.Background(), disco)
	assert.Empty(t, got)
	assert.False(t, complete)

	got, complete = discoverResources(context.Background(), nil)
	assert.Empty(t, got)
	assert.False(t, complete)
}

func TestSourceAccessListsMutatingPolicyOnce(t *testing.T) {
	count := 0
	for _, a := range SourceAccess() {
		if a.Resource.Name == "mutatingadmissionpolicies" &&
			a.Verb == "watch" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
