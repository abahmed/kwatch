package architecture

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/rbac"
)

// customStatusGroups are granted for custom resource status enrichment.
// The dynamic source discovers them at runtime, so they are not part of
// the static access list.
var customStatusGroups = map[string]bool{
	"snapshot.storage.k8s.io":   true,
	"gateway.networking.k8s.io": true,
}

type grant struct {
	namespace, group, resource, url, verb string
}

func manifestGrants(
	t *testing.T, documents []unstructured.Unstructured, installNS string,
) map[grant]bool {
	t.Helper()
	grants := map[grant]bool{}
	for _, doc := range documents {
		namespace := ""
		switch doc.GetKind() {
		case "ClusterRole":
		case "Role":
			namespace = doc.GetNamespace()
			if namespace == "" {
				namespace = installNS
			}
		default:
			continue
		}
		rules, _, _ := unstructured.NestedSlice(doc.Object, "rules")
		for _, raw := range rules {
			rule := raw.(map[string]interface{})
			verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
			groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
			resources, _, _ := unstructured.NestedStringSlice(rule,
				"resources")
			urls, _, _ := unstructured.NestedStringSlice(rule,
				"nonResourceURLs")
			for _, verb := range verbs {
				for _, url := range urls {
					grants[grant{url: url, verb: verb}] = true
				}
				for _, group := range groups {
					for _, resource := range resources {
						grants[grant{namespace, group, resource, "",
							verb}] = true
					}
				}
			}
		}
	}
	return grants
}

func granted(grants map[grant]bool, access kube.Access) bool {
	if access.NonResourceURL != "" {
		return grants[grant{url: access.NonResourceURL, verb: access.Verb}]
	}
	cluster := grant{"", access.Resource.Group, access.Resource.Name, "",
		access.Verb}
	if grants[cluster] {
		return true
	}
	cluster.namespace = access.Namespace
	return access.Namespace != "" && grants[cluster]
}

func assertLeastPrivilegeRBAC(
	t *testing.T, documents []unstructured.Unstructured, installNS string,
) {
	t.Helper()
	grants := manifestGrants(t, documents, installNS)
	checks := rbac.Checks(installNS, true)
	// The audit itself creates access reviews.
	used := map[[2]string]bool{
		{"authorization.k8s.io", "selfsubjectaccessreviews"}: true,
	}
	for _, access := range checks {
		used[[2]string{access.Resource.Group, access.Resource.Name}] = true
		if !granted(grants, access) {
			t.Errorf("manifest does not grant %+v", access)
		}
	}
	for g := range grants {
		if g.url != "" || customStatusGroups[g.group] {
			continue
		}
		if !used[[2]string{g.group, g.resource}] {
			t.Errorf("manifest grants unused %s %s/%s", g.verb, g.group,
				g.resource)
		}
	}
}

func TestRawManifestRBACIsLeastPrivilege(t *testing.T) {
	assertLeastPrivilegeRBAC(t,
		readManifestFile(t, "../../deploy/deploy.yaml"), "kwatch")
}

func TestHelmRBACIsLeastPrivilege(t *testing.T) {
	assertLeastPrivilegeRBAC(t,
		renderHelm(t, "../../deploy/chart", "--namespace", "kwatch"),
		"kwatch")
}
