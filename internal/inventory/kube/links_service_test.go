package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func clusterObject(
	apiVersion, kind string, content map[string]any,
) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]any{"name": "x"},
	}
	for key, value := range content {
		obj[key] = value
	}
	return &unstructured.Unstructured{Object: obj}
}

func serviceClient(ns, name string) map[string]any {
	return map[string]any{"service": map[string]any{
		"namespace": ns, "name": name,
	}}
}

func TestServedByLinksReadServiceReferences(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "sys", "hook")
	tt := []struct {
		name       string
		group      string
		apiVersion string
		kind       string
		content    map[string]any
		want       []inventory.EntityID
	}{
		{"crd_conversion", "apiextensions.k8s.io",
			"apiextensions.k8s.io/v1", "CustomResourceDefinition",
			map[string]any{"spec": map[string]any{
				"conversion": map[string]any{"webhook": map[string]any{
					"clientConfig": serviceClient("sys", "hook"),
				}},
			}}, []inventory.EntityID{svc}},
		{"apiservice", "apiregistration.k8s.io",
			"apiregistration.k8s.io/v1", "APIService",
			map[string]any{"spec": serviceClient("sys", "hook")},
			[]inventory.EntityID{svc}},
		{"mutating_webhook", "admissionregistration.k8s.io",
			"admissionregistration.k8s.io/v1",
			"MutatingWebhookConfiguration",
			map[string]any{"webhooks": []any{
				map[string]any{"clientConfig": serviceClient("sys", "hook")},
				map[string]any{"clientConfig": map[string]any{
					"url": "https://example.test",
				}},
			}}, []inventory.EntityID{svc}},
		{"validating_webhook", "admissionregistration.k8s.io",
			"admissionregistration.k8s.io/v1",
			"ValidatingWebhookConfiguration",
			map[string]any{"webhooks": []any{
				map[string]any{"clientConfig": serviceClient("sys", "hook")},
			}}, []inventory.EntityID{svc}},
		{"service_without_namespace", "apiregistration.k8s.io",
			"apiregistration.k8s.io/v1", "APIService",
			map[string]any{"spec": serviceClient("", "hook")}, nil},
		{"wrong_group", "example.com", "example.com/v1", "APIService",
			map[string]any{"spec": serviceClient("sys", "hook")}, nil},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s := kube.NewUnstructuredSchema(tc.group, tc.kind)
			d, ok := s.Describe(
				clusterObject(tc.apiVersion, tc.kind, tc.content))
			require.True(t, ok)
			assert.Equal(t, tc.want, d.Relations[inventory.Serves])
		})
	}
}

func TestLinksServedByOnlyPresentService(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	schema := kube.NewUnstructuredSchema("apiregistration.k8s.io",
		"APIService")
	feed(t, model, schema, clusterObject("apiregistration.k8s.io/v1",
		"APIService", map[string]any{
			"spec": serviceClient("sys", "metrics"),
		}))
	id := inventory.CoreID(kube.KindAPIService, "", "x")
	assert.Empty(t, linksOf(model, id, inventory.Serves))

	feed(t, model, kube.ServiceSchema{}, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics", Namespace: "sys"},
	})
	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindService, "sys", "metrics"),
	}, linksOf(model, id, inventory.Serves))
}
