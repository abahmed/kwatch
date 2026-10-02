package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func object(kind string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": kind,
		"metadata": map[string]any{
			"name": "x", "namespace": "ns", "generation": int64(2),
			"ownerReferences": []any{map[string]any{
				"kind": "Deployment", "name": "d", "controller": true,
			}},
		},
		"spec": spec,
		"status": map[string]any{
			"readyToUse": true,
			"error":      map[string]any{"message": "boom"},
			"conditions": []any{
				map[string]any{
					"type": "Ready", "status": "False", "reason": "Bad",
					"lastTransitionTime": "2026-01-02T03:04:05Z",
				},
				map[string]any{"type": "a.b/c", "status": "True"},
				"junk",
			},
		},
	}}
}

func TestUnstructuredSchemaDescribesCommonFields(t *testing.T) {
	s := kube.NewUnstructuredSchema("example.com", "Widget")
	assert.Equal(t, inventory.Kind("widget"), s.Kind())
	d, ok := s.Describe(object("Widget", nil))
	assert.True(t, ok)
	custom, _ := d.Attributes[kube.AttrCustom].AsBool()
	assert.True(t, custom)
	assert.Equal(t, "False", text(d, kube.ConditionKey("Ready")))
	assert.Equal(t, "boom", text(d, kube.AttrMessage))
	ready, _ := d.Attributes[kube.AttrReady].AsBool()
	assert.True(t, ready)
	assert.Len(t, d.Relations[inventory.OwnedBy], 1)
	_, hasDotted := d.Attributes[kube.ConditionKey("a.b/c")]
	assert.False(t, hasDotted)
	_, ok = s.Describe(object("Other", nil))
	assert.False(t, ok)
}

func TestUnstructuredSchemaLinksWellKnownKinds(t *testing.T) {
	tt := []struct {
		name string
		kind string
		spec map[string]any
		rel  inventory.RelationType
		want inventory.EntityID
	}{
		{"snapshot", "VolumeSnapshot", map[string]any{
			"source": map[string]any{"persistentVolumeClaimName": "data"},
		}, inventory.References,
			inventory.CoreID(kube.KindPVC, "ns", "data")},
		{"route_backend", "HTTPRoute", map[string]any{
			"rules": []any{map[string]any{"backendRefs": []any{
				map[string]any{"name": "web"},
			}}},
		}, inventory.RoutesTo,
			inventory.CoreID(kube.KindService, "ns", "web")},
		{"route_parent", "HTTPRoute", map[string]any{
			"parentRefs": []any{map[string]any{
				"name": "gw", "namespace": "infra", "kind": "Gateway",
			}},
		}, inventory.References,
			inventory.NewEntityID("gateway.networking.k8s.io", "gateway",
				"infra", "gw")},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s := kube.NewUnstructuredSchema("example.com", tc.kind)
			d, ok := s.Describe(object(tc.kind, tc.spec))
			assert.True(t, ok)
			assert.Contains(t, d.Relations[tc.rel], tc.want)
		})
	}
}

func TestUnstructuredSchemaDiffTracksGeneration(t *testing.T) {
	s := kube.NewUnstructuredSchema("example.com", "Widget")
	before, after := object("Widget", nil), object("Widget", nil)
	assert.Nil(t, s.Diff(before, after))
	after.SetGeneration(3)
	got := s.Diff(before, after)
	assert.Len(t, got, 1)
	assert.Equal(t, "generation 3", got[0].After)
	assert.Nil(t, s.Diff(before, "x"))
}
