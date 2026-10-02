package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStatusTransformKeepsStatusAndReferences(t *testing.T) {
	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
		"metadata": map[string]any{
			"name": "r", "namespace": "ns",
			"annotations": map[string]any{
				lastAppliedAnnotation: "{}", "keep": "yes",
			},
		},
		"spec": map[string]any{
			"hostnames":  []any{"a.example.com"},
			"parentRefs": []any{map[string]any{"name": "gw"}},
			"rules": []any{map[string]any{
				"matches":     []any{map[string]any{"path": "/"}},
				"backendRefs": []any{map[string]any{"name": "svc"}},
			}},
			"service":  map[string]any{"name": "s", "namespace": "n"},
			"template": map[string]any{"spec": map[string]any{}},
			"replicas": int64(3),
			"blob":     strings.Repeat("x", maxMessageLength+1),
		},
		"status": map[string]any{"phase": "Ready"},
		"data":   map[string]any{"secret": "value"},
	}}

	out, err := statusTransform(route)

	require.NoError(t, err)
	u := out.(*unstructured.Unstructured)
	assert.Equal(t, map[string]any{
		"parentRefs": []any{map[string]any{"name": "gw"}},
		"rules": []any{map[string]any{
			"backendRefs": []any{map[string]any{"name": "svc"}},
		}},
		"service":  map[string]any{"name": "s", "namespace": "n"},
		"replicas": int64(3),
	}, u.Object["spec"])
	assert.Equal(t, map[string]any{"phase": "Ready"}, u.Object["status"])
	assert.NotContains(t, u.Object, "data")
	assert.Equal(t, map[string]string{"keep": "yes"}, u.GetAnnotations())
	assert.Equal(t, "HTTPRoute", u.GetKind())
	assert.Equal(t, "r", u.GetName())
}

func TestStatusTransformPassesOtherObjects(t *testing.T) {
	obj := "not an object"
	out, err := statusTransform(obj)
	require.NoError(t, err)
	assert.Equal(t, obj, out)
}

func TestMetadataTransformDropsLastApplied(t *testing.T) {
	m := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
		Name: "l", Annotations: map[string]string{
			lastAppliedAnnotation: "{}",
		},
	}}
	out, err := metadataTransform(m)
	require.NoError(t, err)
	assert.Empty(t, out.(*metav1.PartialObjectMetadata).Annotations)
}

func TestPruneRulesIgnoresMalformedRules(t *testing.T) {
	assert.Nil(t, pruneRules("x"))
	assert.Equal(t, []any{map[string]any{}},
		pruneRules([]any{map[string]any{"matches": []any{}}}))
}

// A custom resource with inline credentials must not keep them in a
// status cache, while references to Secrets stay.
func TestStatusTransformDropsSecretNamedSpecFields(t *testing.T) {
	datasource := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "grafana.example.com/v1", "kind": "Datasource",
		"metadata": map[string]any{"name": "prom", "namespace": "ns"},
		"spec": map[string]any{
			"url":            "http://prometheus:9090",
			"password":       "hunter2",
			"apiKey":         "abc123",
			"bearer_token":   "t0k3n",
			"privateKey":     "-----BEGIN KEY-----",
			"secureJsonData": map[string]any{"httpHeaderValue1": "x"},
			"basicAuth": map[string]any{
				"user": "admin", "clientSecret": "s3cr3t",
			},
			"secretRef":       map[string]any{"name": "prom-creds"},
			"tokenSecretName": "prom-token",
		},
	}}

	out, err := statusTransform(datasource)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"url":             "http://prometheus:9090",
		"basicAuth":       map[string]any{"user": "admin"},
		"secretRef":       map[string]any{"name": "prom-creds"},
		"tokenSecretName": "prom-token",
	}, out.(*unstructured.Unstructured).Object["spec"])
}
