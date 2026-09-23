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
