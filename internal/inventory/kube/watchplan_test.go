package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func planned(group, resource string, tier int) plannedResource {
	return plannedResource{
		gvr: schema.GroupVersionResource{
			Group: group, Version: "v1", Resource: resource,
		},
		tier: tier,
	}
}

func TestApplyBudgetKeepsPriorityOrder(t *testing.T) {
	resources := []plannedResource{
		planned("zeta.example.com", "things", tierCustom),
		planned("gateway.networking.k8s.io", "httproutes", tierPreferred),
		planned("alpha.example.com", "things", tierCustom),
		planned("cert-manager.io", "certificates", tierOperator),
		planned("coordination.k8s.io", "leases", tierBuiltin),
		planned(crdResource.Group, crdResource.Resource, tierAnchor),
	}

	kept, skipped := applyBudget(resources, 5, notRunning)

	assert.Equal(t, []string{"zeta.example.com"}, groups(skipped))
	assert.Equal(t, []string{
		crdResource.Group, "coordination.k8s.io",
		"gateway.networking.k8s.io", "cert-manager.io",
		"alpha.example.com",
	}, groups(kept))
}

func TestApplyBudgetKeepsRunningTypes(t *testing.T) {
	resources := []plannedResource{
		planned("gateway.networking.k8s.io", "httproutes", tierPreferred),
		planned("zeta.example.com", "things", tierCustom),
		planned(crdResource.Group, crdResource.Resource, tierAnchor),
	}
	running := func(gvr schema.GroupVersionResource) bool {
		return gvr.Group == "zeta.example.com"
	}

	kept, skipped := applyBudget(resources, 2, running)

	assert.Equal(t, []string{crdResource.Group, "zeta.example.com"},
		groups(kept), "a new type must not push out a running one")
	assert.Equal(t, []string{"gateway.networking.k8s.io"}, groups(skipped))
}

func TestApplyBudgetWithinBudgetSkipsNothing(t *testing.T) {
	kept, skipped := applyBudget(
		[]plannedResource{planned("a", "b", tierCustom)}, 5, notRunning)
	assert.Len(t, kept, 1)
	assert.Empty(t, skipped)
}

func notRunning(schema.GroupVersionResource) bool { return false }

func groups(resources []plannedResource) []string {
	var out []string
	for _, r := range resources {
		out = append(out, r.gvr.Group)
	}
	return out
}

func TestOperatorGroupMatchesSubgroups(t *testing.T) {
	for group, want := range map[string]bool{
		"cert-manager.io":                true,
		"acme.cert-manager.io":           true,
		"source.toolkit.fluxcd.io":       true,
		"argoproj.io":                    true,
		"keda.sh":                        true,
		"karpenter.sh":                   true,
		"external-secrets.io":            true,
		"notcert-manager.io":             false,
		"example.com":                    false,
		"generators.external-secrets.io": true,
	} {
		assert.Equal(t, want, operatorGroup(group), group)
	}
}

func TestPlanResourceTiersAndModes(t *testing.T) {
	typed := typedResources()
	tt := []struct {
		name      string
		gvr       schema.GroupVersionResource
		hasStatus bool
		watched   bool
		tier      int
		mode      WatchMode
	}{
		{"typed_kind_keeps_typed_informer",
			schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
			false, false, 0, ""},
		{"typed_service_account",
			schema.GroupVersionResource{Version: "v1",
				Resource: "serviceaccounts"}, false, false, 0, ""},
		{"crd_anchor", crdResource, true, true, tierAnchor, WatchStatus},
		{"apiservice_any_version", schema.GroupVersionResource{
			Group: "apiregistration.k8s.io", Version: "v1beta1",
			Resource: "apiservices"}, true, true, tierAnchor, WatchStatus},
		{"lease_metadata", schema.GroupVersionResource{
			Group: "coordination.k8s.io", Version: "v1",
			Resource: "leases"}, false, true, tierBuiltin, WatchMetadata},
		{"metadata_kind_even_with_status", schema.GroupVersionResource{
			Group: "apps", Version: "v1", Resource: "controllerrevisions"},
			true, true, tierBuiltin, WatchMetadata},
		{"builtin_status", schema.GroupVersionResource{
			Group: "certificates.k8s.io", Version: "v1",
			Resource: "certificatesigningrequests"},
			true, true, tierBuiltin, WatchStatus},
		{"gateway_preferred", schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Resource: "gateways"}, true, true, tierPreferred, WatchStatus},
		{"snapshot_preferred", schema.GroupVersionResource{
			Group: "snapshot.storage.k8s.io", Version: "v1",
			Resource: "volumesnapshots"}, false, true, tierPreferred,
			WatchStatus},
		{"operator_group", schema.GroupVersionResource{
			Group: "kustomize.toolkit.fluxcd.io", Version: "v1",
			Resource: "kustomizations"}, true, true, tierOperator,
			WatchStatus},
		{"custom_without_status", schema.GroupVersionResource{
			Group: "example.com", Version: "v1", Resource: "plains"},
			false, true, tierCustom, WatchStatus},
		{"endpoints_excluded", schema.GroupVersionResource{
			Version: "v1", Resource: "endpoints"}, false, false, 0, ""},
		{"kwatchconfig_excluded", schema.GroupVersionResource{
			Group: "kwatch.abahmed.dev", Version: "v1alpha1",
			Resource: "kwatchconfigs"}, true, false, 0, ""},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := planResource(tc.gvr, "Kind", tc.hasStatus, typed)
			assert.Equal(t, tc.watched, ok)
			if ok {
				assert.Equal(t, tc.tier, p.tier)
				assert.Equal(t, tc.mode, p.mode)
			}
		})
	}
}

func TestAuditedDynamicResourcesCoverMetadataKinds(t *testing.T) {
	audited := map[Resource]bool{}
	for _, r := range auditedDynamicResources() {
		audited[r] = true
	}
	for r := range metadataResources {
		assert.True(t, audited[r], r.Name)
	}
	assert.True(t, audited[Resource{Group: crdResource.Group,
		Name: crdResource.Resource}])
}
