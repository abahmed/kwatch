package architecture

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rbac"
)

// rbacManifests renders every supported install: the raw manifest and
// both chart modes, with extra --set values for the chart.
func rbacManifests(
	t *testing.T, set ...string,
) map[string][]unstructured.Unstructured {
	t.Helper()
	out := map[string][]unstructured.Unstructured{}
	for _, mode := range []string{"full", "least-privilege"} {
		args := []string{"--namespace", "kwatch",
			"--set", "rbac.mode=" + mode}
		for _, value := range set {
			args = append(args, "--set", value)
		}
		out["helm "+mode] = renderHelm(t, "../../deploy/chart", args...)
	}
	if len(set) == 0 {
		out["raw"] = readManifestFile(t, "../../deploy/deploy.yaml")
	}
	return out
}

func TestManifestsReadKubeletWithoutNodesProxy(t *testing.T) {
	for name, documents := range rbacManifests(t) {
		grants := manifestGrants(t, documents, "kwatch")
		for _, resource := range []string{"nodes/stats", "nodes/metrics"} {
			if !grants[grant{"", "", resource, "", "get", ""}] {
				t.Errorf("%s: get %s is not granted", name, resource)
			}
		}
		for g := range grants {
			if g.resource == "nodes/proxy" {
				t.Errorf("%s grants %s nodes/proxy", name, g.verb)
			}
		}
	}
}

func TestManifestsScopeLeaseGetAndUpdateByName(t *testing.T) {
	leases := map[string]string{
		"raw": rawLease, "helm full": helmLease,
		"helm least-privilege": helmLease,
	}
	for name, documents := range rbacManifests(t) {
		grants := manifestGrants(t, documents, "kwatch")
		for g := range grants {
			if g.resource != "leases" || g.namespace != "kwatch" {
				continue
			}
			switch g.verb {
			case "get", "update":
				if g.name != leases[name] {
					t.Errorf("%s: %s on leases is not limited to %q",
						name, g.verb, leases[name])
				}
			}
		}
		if !grants[grant{"kwatch", "coordination.k8s.io", "leases", "",
			"create", ""}] {
			t.Errorf("%s: Lease create is missing", name)
		}
	}
}

// watch.secrets=false removes every Secret permission in both modes,
// while every other permission kwatch audits stays granted.
func TestHelmWatchSecretsFalseGrantsNoSecretAccess(t *testing.T) {
	for name, documents := range rbacManifests(t, "watch.secrets=false") {
		grants := manifestGrants(t, documents, "kwatch")
		for g := range grants {
			if licenseGrant(g, "kwatch") {
				continue
			}
			if g.resource == "secrets" ||
				(g.resource == "*" && (g.group == "" || g.group == "*")) {
				t.Errorf("%s grants %s on %s/%s", name, g.verb, g.group,
					g.resource)
			}
		}
		checks := kube.WithoutResource(
			rbac.Checks("kwatch", helmLease, true), kube.SecretsResource)
		for _, access := range checks {
			if !granted(grants, access) {
				t.Errorf("%s does not grant %+v", name, access)
			}
		}
	}
}

// Without Secret watching and without a license key the chart grants no
// Secret permission at all.
func TestKeylessHelmWithoutSecretWatchingGrantsNoSecretAccess(t *testing.T) {
	for name, documents := range rbacManifests(t, "watch.secrets=false") {
		for g := range manifestGrants(t, documents, "kwatch") {
			if g.resource == "secrets" {
				t.Errorf("%s: unexpected Secret grant %+v", name, g)
			}
		}
	}
}

// With a license key and without Secret watching, the only Secret
// permissions left are the license instance Secret's: get and update on
// that one name, and no create.
func TestSecretGrantsAreOnlyTheLicenseInstanceSecret(t *testing.T) {
	want := map[grant]bool{
		{"kwatch", "", "secrets", "", "get", licenseInstanceSecret}:    true,
		{"kwatch", "", "secrets", "", "update", licenseInstanceSecret}: true,
	}
	for name, documents := range rbacManifests(
		t, "watch.secrets=false", "license.key=KW1-x") {
		got := map[grant]bool{}
		for g := range manifestGrants(t, documents, "kwatch") {
			if g.resource == "secrets" {
				got[g] = true
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: secret grants %v, want %v", name, got, want)
		}
		for g := range want {
			if !got[g] {
				t.Errorf("%s: missing %+v", name, g)
			}
		}
	}
	// The raw manifest grants the same.
	raw := manifestGrants(t,
		readManifestFile(t, "../../deploy/deploy.yaml"), "kwatch")
	for g := range want {
		if !raw[g] {
			t.Errorf("raw manifest: missing %+v", g)
		}
	}
}

func TestHelmWatchSecretsFalseDisablesTheSecretWatch(t *testing.T) {
	documents := renderHelm(t, "../../deploy/chart",
		"--set", "watch.secrets=false")
	deployment := findManifestKind(t, documents, "Deployment")
	if !hasEnv(t, deployment, "KWATCH_WATCH_SECRETS", "false") {
		t.Error("KWATCH_WATCH_SECRETS=false is not passed to kwatch")
	}
	documents = renderHelm(t, "../../deploy/chart")
	deployment = findManifestKind(t, documents, "Deployment")
	if hasEnv(t, deployment, "KWATCH_WATCH_SECRETS", "false") {
		t.Error("Secrets must be watched by default")
	}
}

// hasEnv reports whether the first container sets name to value.
func hasEnv(
	t *testing.T, deployment *unstructured.Unstructured, name, value string,
) bool {
	t.Helper()
	containers, _, _ := unstructured.NestedSlice(deployment.Object,
		"spec", "template", "spec", "containers")
	if len(containers) == 0 {
		t.Fatal("deployment has no containers")
	}
	env, _, _ := unstructured.NestedSlice(
		containers[0].(map[string]interface{}), "env")
	for _, raw := range env {
		entry := raw.(map[string]interface{})
		if entry["name"] == name && entry["value"] == value {
			return true
		}
	}
	return false
}

// A Community install has no license key, so the chart renders neither the
// license Role nor the instance Secret it reads; any way of configuring a
// key renders both, and the Role covers what the audit checks for it.
func TestHelmRendersTheLicenseRoleOnlyWithAKey(t *testing.T) {
	hasRole := func(docs []unstructured.Unstructured) bool {
		for _, doc := range docs {
			if doc.GetKind() == "Role" &&
				strings.HasSuffix(doc.GetName(), "-license") {
				return true
			}
		}
		return false
	}
	hasSecret := func(docs []unstructured.Unstructured) bool {
		for _, doc := range docs {
			if doc.GetKind() == "Secret" &&
				doc.GetName() == licenseInstanceSecret {
				return true
			}
		}
		return false
	}
	none := renderHelm(t, "../../deploy/chart", "--namespace", "kwatch")
	if hasRole(none) || hasSecret(none) {
		t.Error("a keyless install must not render the license Role " +
			"or the instance Secret")
	}
	for _, set := range []string{
		"license.key=KW1-x", "license.existingSecret=mine",
		"license.instanceSecret=true",
	} {
		docs := renderHelm(t, "../../deploy/chart", "--namespace", "kwatch",
			"--set", set)
		if !hasRole(docs) || !hasSecret(docs) {
			t.Errorf("%s: the license Role and instance Secret are "+
				"missing", set)
			continue
		}
		grants := manifestGrants(t, docs, "kwatch")
		for _, access := range rbac.LicenseChecks(
			"kwatch", licenseInstanceSecret) {
			if !granted(grants, access) {
				t.Errorf("%s: the Role does not grant %+v", set, access)
			}
		}
	}
}
