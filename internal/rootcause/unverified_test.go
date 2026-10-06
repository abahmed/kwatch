package rootcause

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestUnverifiedScopeNamesTheKindAndNamespace(t *testing.T) {
	tests := map[string]struct {
		id   inventory.EntityID
		want string
	}{
		"namespaced known kind": {
			inventory.CoreID(kube.KindSecret, "billing", "db"),
			"secrets in billing"},
		"known kind with a two-word plural": {
			inventory.CoreID(kube.KindConfigMap, "shop", "cfg"),
			"config maps in shop"},
		"cluster scoped": {
			inventory.CoreID(kube.KindNode, "", "n1"), "nodes"},
		"unlisted kind takes an s": {
			inventory.CoreID(kube.KindDeployment, "shop", "api"),
			"deployments in shop"},
		"kind ending in ss": {
			inventory.CoreID(kube.KindIngress, "shop", "web"),
			"ingresses in shop"},
		"class kinds": {
			inventory.CoreID(kube.KindStorageClass, "", "fast"),
			"storageclasses"},
		"priority class": {
			inventory.CoreID(kube.KindPriorityClass, "", "high"),
			"priorityclasses"},
		"runtime class": {
			inventory.CoreID(kube.KindRuntimeClass, "", "gvisor"),
			"runtimeclasses"},
		"ingress class": {
			inventory.CoreID(kube.KindIngressClass, "", "nginx"),
			"ingressclasses"},
		"consonant and y": {
			inventory.CoreID("registry", "", "r"), "registries"},
		"vowel and y": {
			inventory.CoreID("gateway", "shop", "g"), "gateways in shop"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, UnverifiedScope(tc.id))
		})
	}
}

func TestMergeUnverifiedSortsAndDropsDuplicatesAndBlanks(t *testing.T) {
	got := MergeUnverified(
		[]string{"nodes", "secrets in shop", ""},
		nil,
		[]string{"secrets in shop", "config maps in shop"})

	assert.Equal(t,
		[]string{"config maps in shop", "nodes", "secrets in shop"}, got)
	assert.Nil(t, MergeUnverified(nil, []string{""}))
}
