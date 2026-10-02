package architecture

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rbac"
)

// customStatusGroups are granted for custom resource status enrichment.
// The dynamic source discovers them at runtime, so they are not part of
// the static access list.
var customStatusGroups = map[string]bool{
	"snapshot.storage.k8s.io":   true,
	"gateway.networking.k8s.io": true,
}

// grant is one permission a manifest gives. name is set when the rule
// is limited with resourceNames; empty means every object.
type grant struct {
	namespace, group, resource, url, verb, name string
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
			names, _, _ := unstructured.NestedStringSlice(rule,
				"resourceNames")
			if len(names) == 0 {
				names = []string{""}
			}
			for _, verb := range verbs {
				for _, url := range urls {
					grants[grant{url: url, verb: verb}] = true
				}
				for _, group := range groups {
					for _, resource := range resources {
						for _, name := range names {
							grants[grant{namespace, group, resource, "",
								verb, name}] = true
						}
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
	for _, ns := range []string{"", access.Namespace} {
		for _, group := range []string{access.Resource.Group, "*"} {
			for _, resource := range []string{access.Resource.Name, "*"} {
				for _, name := range []string{"", access.Name} {
					if grants[grant{ns, group, resource, "",
						access.Verb, name}] {
						return true
					}
				}
			}
		}
	}
	return false
}

func assertGrantsCoverChecks(
	t *testing.T, grants map[grant]bool, installNS, lease string,
) {
	t.Helper()
	for _, access := range rbac.Checks(installNS, lease, true) {
		if !granted(grants, access) {
			t.Errorf("manifest does not grant %+v", access)
		}
	}
}

// writeAllowed are the only non-read grants: the Lease kwatch holds and
// the access reviews the RBAC audit creates.
func writeAllowed(g grant, installNS string) bool {
	if g.group == "authorization.k8s.io" &&
		g.resource == "selfsubjectaccessreviews" && g.verb == "create" {
		return true
	}
	// Update must be limited to kwatch's own Lease by name; create cannot
	// be limited by name in RBAC.
	return g.group == "coordination.k8s.io" && g.resource == "leases" &&
		g.namespace == installNS &&
		(g.verb == "create" || (g.verb == "update" && g.name != ""))
}

func assertFullRBACIsReadOnly(
	t *testing.T, documents []unstructured.Unstructured,
	installNS, lease string,
) {
	t.Helper()
	grants := manifestGrants(t, documents, installNS)
	assertGrantsCoverChecks(t, grants, installNS, lease)
	if !grants[grant{"", "*", "*", "", "list", ""}] {
		t.Error("full mode must grant list on every resource")
	}
	for g := range grants {
		if g.verb == "get" && (g.group == "*" || g.resource == "*") {
			// A wildcard get also matches subresources such as
			// pods/exec, pods/attach and nodes/proxy.
			t.Errorf("full mode grants wildcard get on %s/%s",
				g.group, g.resource)
		}
		switch g.verb {
		case "get", "list", "watch":
		default:
			if !writeAllowed(g, installNS) {
				t.Errorf("full mode grants write %s %s/%s", g.verb,
					g.group, g.resource)
			}
		}
	}
}

// extraLeastPrivilege are grants outside SourceAccess: access reviews,
// KwatchConfig watches and custom status enrichment.
func extraLeastPrivilege(g grant) bool {
	switch {
	case g.url != "":
		return true
	case customStatusGroups[g.group]:
		return true
	case g.group == "authorization.k8s.io":
		return g.resource == "selfsubjectaccessreviews" &&
			g.verb == "create"
	case g.group == "kwatch.abahmed.dev":
		return g.resource == "kwatchconfigs"
	}
	return false
}

func assertLeastPrivilegeRBAC(
	t *testing.T, documents []unstructured.Unstructured,
	installNS, lease string,
) {
	t.Helper()
	grants := manifestGrants(t, documents, installNS)
	assertGrantsCoverChecks(t, grants, installNS, lease)
	expected := map[grant]bool{}
	for _, a := range rbac.Checks(installNS, lease, true) {
		if a.NonResourceURL == "" {
			expected[grant{a.Namespace, a.Resource.Group,
				a.Resource.Name, "", a.Verb, a.Name}] = true
		}
	}
	for g := range grants {
		if !expected[g] && !extraLeastPrivilege(g) {
			t.Errorf("manifest grants unused %s %s/%s in %q", g.verb,
				g.group, g.resource, g.namespace)
		}
	}
}

// Leader-election Lease names: the raw manifest's, and the chart's for
// the release renderHelm uses.
const (
	rawLease  = "kwatch-leader"
	helmLease = "architecture-kwatch-leader"
)

func TestRawManifestRBACIsReadOnly(t *testing.T) {
	assertFullRBACIsReadOnly(t,
		readManifestFile(t, "../../deploy/deploy.yaml"), "kwatch", rawLease)
}

func TestHelmFullRBACIsReadOnly(t *testing.T) {
	assertFullRBACIsReadOnly(t,
		renderHelm(t, "../../deploy/chart", "--namespace", "kwatch"),
		"kwatch", helmLease)
}

func TestHelmLeastPrivilegeRBACMatchesSourceAccess(t *testing.T) {
	assertLeastPrivilegeRBAC(t,
		renderHelm(t, "../../deploy/chart", "--namespace", "kwatch",
			"--set", "rbac.mode=least-privilege"),
		"kwatch", helmLease)
}

func TestHelmLeastPrivilegeRBACHasNoWildcards(t *testing.T) {
	grants := manifestGrants(t,
		renderHelm(t, "../../deploy/chart", "--namespace", "kwatch",
			"--set", "rbac.mode=least-privilege"), "kwatch")
	for g := range grants {
		if g.group == "*" || g.resource == "*" {
			t.Errorf("least-privilege grants wildcard %+v", g)
		}
	}
}
