package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// feed describes obj with schema and applies it to model.
func feed(
	t *testing.T, model *inventory.Model, schema kube.Schema, obj any,
) {
	t.Helper()
	observations := kube.NewTranslator(schema).Added(obj, true, fixedTime())
	require.NotEmpty(t, observations)
	for _, o := range observations {
		_, err := model.Apply(o)
		require.NoError(t, err)
	}
}

func widget(spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]any{
			"name": "w", "namespace": testNamespace,
		},
		"spec": spec,
	}}
}

func widgetSchema() kube.Schema {
	return kube.NewUnstructuredSchema("example.com", "Widget")
}

func widgetID() inventory.EntityID {
	return inventory.NewEntityID("example.com", "widget", testNamespace, "w")
}

func linksOf(
	model *inventory.Model, id inventory.EntityID,
	rel inventory.RelationType,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, link := range kube.Links(model, id) {
		if link.Type == rel {
			out = append(out, link.To)
		}
	}
	return out
}

func TestUseLinksReadConventionalFields(t *testing.T) {
	ns := testNamespace
	tt := []struct {
		name string
		spec map[string]any
		want []inventory.EntityID
	}{
		{"secret_name", map[string]any{"secretName": "s"},
			[]inventory.EntityID{inventory.CoreID(kube.KindSecret, ns, "s")}},
		{"nested_config_map_name", map[string]any{
			"template": map[string]any{"configMapName": "c"},
		}, []inventory.EntityID{
			inventory.CoreID(kube.KindConfigMap, ns, "c")}},
		{"service_account_name", map[string]any{
			"serviceAccountName": "sa",
		}, []inventory.EntityID{
			inventory.CoreID(kube.KindAccount, ns, "sa")}},
		{"cluster_scoped_classes", map[string]any{
			"storageClassName": "fast", "ingressClassName": "nginx",
			"priorityClassName": "high", "runtimeClassName": "gvisor",
		}, []inventory.EntityID{
			inventory.CoreID(kube.KindIngressClass, "", "nginx"),
			inventory.CoreID(kube.KindPriorityClass, "", "high"),
			inventory.CoreID(kube.KindRuntimeClass, "", "gvisor"),
			inventory.CoreID(kube.KindStorageClass, "", "fast"),
		}},
		{"secret_ref_without_kind", map[string]any{
			"auth": map[string]any{"secretRef": map[string]any{"name": "s"}},
		}, []inventory.EntityID{inventory.CoreID(kube.KindSecret, ns, "s")}},
		{"typed_ref_with_group", map[string]any{"issuerRef": map[string]any{
			"name": "ca", "kind": "ClusterIssuer", "group": "cert-manager.io",
		}}, []inventory.EntityID{inventory.NewEntityID(
			"cert-manager.io", "clusterissuer", "", "ca")}},
		{"ref_list_with_api_group", map[string]any{
			"targetRefs": []any{map[string]any{
				"name": "d", "kind": "Deployment", "apiGroup": "apps",
			}},
		}, []inventory.EntityID{
			inventory.CoreID(kube.KindDeployment, ns, "d")}},
		{"unknown_name_field_ignored", map[string]any{
			"hostName": "h", "fooRef": map[string]any{"name": "x"},
		}, nil},
		{"route_refs_left_to_routes", map[string]any{
			"parentRefs": []any{map[string]any{
				"name": "gw", "kind": "Gateway",
			}},
		}, nil},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := widgetSchema().Describe(widget(tc.spec))
			require.True(t, ok)
			assert.ElementsMatch(t, tc.want,
				d.Relations[inventory.References])
		})
	}
}

func TestLinksUsesOnlyPresentTargets(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	feed(t, model, widgetSchema(), widget(map[string]any{
		"secretName": "present", "configMapName": "missing",
		"wrongRef": map[string]any{
			"name": "present", "kind": "Secret", "apiGroup": "example.com",
		},
	}))
	assert.Empty(t, linksOf(model, widgetID(), inventory.References))

	feed(t, model, kube.SecretSchema{}, secret("present"))
	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindSecret, testNamespace, "present"),
	}, linksOf(model, widgetID(), inventory.References),
		"a Secret in another group must not link to the core Secret")
}

func TestLinksSelectPodsByLabels(t *testing.T) {
	tt := []struct {
		name     string
		selector any
		want     []string
	}{
		{"match_labels", map[string]any{
			"matchLabels": map[string]any{"app": "web"},
		}, []string{"web"}},
		{"match_expressions", map[string]any{
			"matchExpressions": []any{map[string]any{
				"key": "app", "operator": "In", "values": []any{"db"},
			}},
		}, []string{"db"}},
		{"plain_map", map[string]any{"app": "web"}, []string{"web"}},
		{"string", "app=db", []string{"db"}},
		{"no_match", map[string]any{"app": "cache"}, nil},
		{"empty_selects_nothing", map[string]any{}, nil},
		{"invalid", map[string]any{"app": int64(1)}, nil},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			for _, app := range []string{"web", "db"} {
				p := pod(app)
				p.Labels = map[string]string{"app": app}
				feed(t, model, kube.PodSchema{}, p)
			}
			other := pod("elsewhere")
			other.Namespace = "other"
			other.Labels = map[string]string{"app": "web"}
			feed(t, model, kube.PodSchema{}, other)
			feed(t, model, widgetSchema(),
				widget(map[string]any{"selector": tc.selector}))

			var got []string
			for _, id := range linksOf(model, widgetID(),
				inventory.Selects) {
				got = append(got, id.Name)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLinksSelectsWithPodSelector(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	p := pod("web")
	p.Labels = map[string]string{"app": "web"}
	feed(t, model, kube.PodSchema{}, p)
	feed(t, model, widgetSchema(), widget(map[string]any{
		"podSelector": map[string]any{
			"matchLabels": map[string]any{"app": "web"},
		},
	}))
	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindPod, testNamespace, "web"),
	}, linksOf(model, widgetID(), inventory.Selects))
}

func deploymentIn(namespace, name string) *appsv1.Deployment {
	d := deployment(name)
	d.Namespace = namespace
	return d
}
