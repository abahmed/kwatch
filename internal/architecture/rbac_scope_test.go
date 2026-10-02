package architecture

import (
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
