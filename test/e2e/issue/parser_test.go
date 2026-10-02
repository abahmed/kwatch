package issue

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAcceptsOnlyMarkedBlocks(t *testing.T) {
	body := "untrusted prose\n" +
		"<!-- kwatch-config -->\n```yaml\n" +
		"app:\n  clusterName: issue\n```\n" +
		"<!-- kwatch-resources -->\n```yaml\n" +
		"apiVersion: v1\nkind: Pod\nmetadata:\n" +
		"  name: broken\n```\n" +
		"<!-- kwatch-expectation -->\nPod should be observed\n"
	got, err := Parse(body)
	require.NoError(t, err)
	require.Contains(t, string(got.ConfigYAML), "clusterName")
	require.Contains(t, string(got.ResourcesYAML), "kind: Pod")
	require.Equal(t, "Pod should be observed", got.Expectation)
}

func TestParseRejectsCommandsAndURLs(t *testing.T) {
	body := "<!-- kwatch-config -->\n```yaml\n" +
		"url: https://example.invalid\n```\n" +
		"<!-- kwatch-resources -->\n```yaml\n" +
		"kind: Pod\n```\n" +
		"<!-- kwatch-expectation -->\nnone\n"
	_, err := Parse(body)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestSanitizeResourcesRewritesNamespaceAndImage(t *testing.T) {
	input := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: broken
spec:
  template:
    spec:
      containers:
      - name: app
        image: user/private:latest
`)
	got, err := SanitizeResources(
		input, "issue-123", "kwatch-e2e-workload:test",
	)
	require.NoError(t, err)
	text := string(got)
	require.Contains(t, text, "namespace: issue-123")
	require.Contains(t, text, "image: kwatch-e2e-workload:test")
	require.NotContains(t, text, "private:latest")
}

func TestSanitizeResourcesRejectsUnsafeObjects(t *testing.T) {
	input := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: unsafe
spec:
  hostNetwork: true
`)
	_, err := SanitizeResources(
		input, "issue-123", "kwatch-e2e-workload:test",
	)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unsafe"))
}

func sanitizePod(spec string) error {
	_, err := SanitizeResources([]byte(
		"apiVersion: v1\nkind: Pod\nmetadata:\n  name: p\n"+spec,
	), "issue-123", "kwatch-e2e-workload:test")
	return err
}

func TestSanitizeResourcesRejectsKindsOutsideAllowlist(t *testing.T) {
	kinds := []string{
		"CustomResourceDefinition", "MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration", "StorageClass", "PriorityClass",
		"APIService", "ValidatingAdmissionPolicy", "Role", "RoleBinding",
		"Secret", "ClusterRole", "Namespace", "List",
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			_, err := SanitizeResources([]byte(
				"apiVersion: v1\nkind: "+kind+"\nmetadata:\n  name: x\n",
			), "issue-123", "kwatch-e2e-workload:test")
			require.Error(t, err)
			require.Contains(t, err.Error(), "not an allowed")
		})
	}
}

func TestSanitizeResourcesAcceptsAllowlistedKinds(t *testing.T) {
	for kind := range allowedKinds {
		t.Run(kind, func(t *testing.T) {
			_, err := SanitizeResources([]byte(
				"apiVersion: v1\nkind: "+kind+"\nmetadata:\n  name: x\n",
			), "issue-123", "kwatch-e2e-workload:test")
			require.NoError(t, err)
		})
	}
}

func TestSanitizeResourcesStripsCommandAndArgs(t *testing.T) {
	got, err := SanitizeResources([]byte(`apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  initContainers:
  - name: init
    image: a
    command: ["sh", "-c", "evil"]
  containers:
  - name: app
    image: b
    command: ["sh"]
    args: ["-c", "evil"]
`), "issue-123", "kwatch-e2e-workload:test")
	require.NoError(t, err)
	require.NotContains(t, string(got), "command")
	require.NotContains(t, string(got), "args")
	require.NotContains(t, string(got), "evil")
}

func TestSanitizeResourcesRejectsPrivilegedFields(t *testing.T) {
	c := "spec:\n  containers:\n  - name: a\n    image: i\n"
	tests := map[string]string{
		"hostPort": c + "    ports:\n    - containerPort: 80\n" +
			"      hostPort: 80\n",
		"capabilities add": c + "    securityContext:\n" +
			"      capabilities:\n        add: [NET_ADMIN]\n",
		"runAsUser root": c + "    securityContext:\n      runAsUser: 0\n",
		"privileged":     c + "    securityContext:\n      privileged: true\n",
		"hostPID":        "spec:\n  hostPID: true\n",
		"hostIPC":        "spec:\n  hostIPC: true\n",
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			err := sanitizePod(spec)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unsafe")
		})
	}
}

func TestSanitizeResourcesAllowsSafeSecurityContext(t *testing.T) {
	require.NoError(t, sanitizePod("spec:\n"+
		"  containers:\n  - name: a\n    image: i\n"+
		"    securityContext:\n      runAsUser: 1000\n"+
		"      capabilities:\n        drop: [ALL]\n"))
}

func TestSanitizeObjectRejectsServiceAccountTokens(t *testing.T) {
	// The input validator already rejects "token:" text; these fields are
	// checked again on the decoded object as a second layer.
	objects := map[string]map[string]interface{}{
		"automount": {"spec": map[string]interface{}{
			"automountServiceAccountToken": true,
		}},
		"projected token": {"spec": map[string]interface{}{
			"volumes": []interface{}{map[string]interface{}{
				"projected": map[string]interface{}{
					"sources": []interface{}{map[string]interface{}{
						"serviceAccountToken": map[string]interface{}{},
					}},
				},
			}},
		}},
	}
	for name, object := range objects {
		t.Run(name, func(t *testing.T) {
			object["kind"] = "Pod"
			err := sanitizeObject(object, "ns", "img")
			require.Error(t, err)
			require.Contains(t, err.Error(), "unsafe")
		})
	}
}
