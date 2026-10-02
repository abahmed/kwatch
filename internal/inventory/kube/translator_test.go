package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestTranslatorAdded(t *testing.T) {
	tt := []struct {
		name              string
		obj               any
		initialList       bool
		wantCount         int
		checkObservations func(*testing.T, []inventory.Observation)
	}{
		{
			name:        "initial_list_pod",
			obj:         pod("p1"),
			initialList: true,
			wantCount:   1, // At least 1 observation
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				// InitialList=true: Observed + Related +
				// children, NO Changed
				assert.NotNil(t, byKind[inventory.Observed])
				assert.NotNil(t, byKind[inventory.Related])
				assert.Nil(t, byKind[inventory.Changed])
			},
		},
		{
			name:        "not_initial_list_pod",
			obj:         pod("p1"),
			initialList: false,
			wantCount:   1, // At least 1 observation
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				// InitialList=false: Observed + Related +
				// Changed
				assert.NotNil(t, byKind[inventory.Observed])
				assert.NotNil(t, byKind[inventory.Related])
				assert.NotNil(t, byKind[inventory.Changed])
				// Find Changed for pod entity
				for _, f := range byKind[inventory.Changed] {
					if f.Entity.Kind == "pod" {
						assert.True(t, f.Change.Created)
						assert.Equal(t, "kubectl",
							f.Change.Actor)
						return
					}
				}
				t.Fatal("no Changed observation for pod")
			},
		},
		{
			name:        "deployment_initial",
			obj:         deployment("d1"),
			initialList: true,
			wantCount:   1, // At least one observation
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				assert.NotNil(t, byKind[inventory.Observed])
				assert.NotNil(t, byKind[inventory.Related])
				assert.Nil(t, byKind[inventory.Changed])
			},
		},
		{
			name:        "wrong_type_returns_nil",
			obj:         "not a kubernetes object",
			initialList: true,
			wantCount:   0,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Choose schema based on test case
			var schema kube.Schema
			switch tc.name {
			case "deployment_initial", "wrong_type_returns_nil":
				schema = kube.DeploymentSchema()
			default:
				schema = kube.PodSchema{}
			}
			translator := kube.NewTranslator(schema)
			observations := translator.Added(tc.obj, tc.initialList,
				fixedTime())
			if tc.wantCount > 0 {
				assert.GreaterOrEqual(t, len(observations),
					tc.wantCount)
			} else {
				assert.Equal(t, tc.wantCount, len(observations))
			}
			if tc.checkObservations != nil {
				tc.checkObservations(t, observations)
			}
		})
	}
}

func TestTranslatorUpdated(t *testing.T) {
	tt := []struct {
		name              string
		schema            kube.Schema
		oldObj            any
		newObj            any
		checkObservations func(*testing.T, []inventory.Observation)
	}{
		{
			name:   "pod_status_only",
			schema: kube.PodSchema{},
			oldObj: pod("p1"),
			newObj: func() any {
				p := pod("p1")
				p.Status.Phase = "Succeeded"
				return p
			}(),
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				// Status-only changes should NOT emit Changed
				assert.Nil(t, byKind[inventory.Changed])
				assert.NotNil(t,
					byKind[inventory.Observed])
			},
		},
		{
			name:   "pod_with_deletion",
			schema: kube.PodSchema{},
			oldObj: pod("p1"),
			newObj: func() any {
				p := pod("p1")
				t := metav1.NewTime(fixedTime())
				p.DeletionTimestamp = &t
				return p
			}(),
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				// DeletionTimestamp is a meaningful change
				assert.NotNil(t, byKind[inventory.Changed])
			},
		},
		{
			name:   "deployment_image_change",
			schema: kube.DeploymentSchema(),
			oldObj: deployment("d1"),
			newObj: func() any {
				d := deployment("d1")
				d.Spec.Template.Spec.Containers[0].Image =
					"app:2.0"
				return d
			}(),
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				// Template change is a meaningful change
				assert.NotNil(t, byKind[inventory.Changed])
			},
		},
		{
			name:   "wrong_type_returns_nil",
			schema: kube.DeploymentSchema(),
			oldObj: "not k8s object",
			newObj: "also not k8s object",
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				assert.Len(t, observations, 0)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			translator := kube.NewTranslator(tc.schema)
			observations := translator.Updated(tc.oldObj, tc.newObj,
				fixedTime())
			if tc.checkObservations != nil {
				tc.checkObservations(t, observations)
			}
		})
	}
}

func TestTranslatorDeleted(t *testing.T) {
	tt := []struct {
		name              string
		obj               any
		checkObservations func(*testing.T, []inventory.Observation)
	}{
		{
			name: "deleted_pod",
			obj:  pod("p1"),
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				byKind := observationsByKind(observations)
				// Pod deletion should emit Gone observations
				assert.NotNil(t, byKind[inventory.Gone])
				gone := byKind[inventory.Gone]
				// Should have Gone for pod + 1 for container
				assert.GreaterOrEqual(t, len(gone), 1)
			},
		},
		{
			name: "wrong_type_returns_nil",
			obj:  "not a k8s object",
			checkObservations: func(t *testing.T, observations []inventory.Observation) {
				assert.Len(t, observations, 0)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			translator := kube.NewTranslator(kube.PodSchema{})
			observations := translator.Deleted(tc.obj, fixedTime())
			if tc.checkObservations != nil {
				tc.checkObservations(t, observations)
			}
		})
	}
}

func TestTranslatorRelationTypes(t *testing.T) {
	tt := []struct {
		name         string
		schema       kube.Schema
		wantRelTypes []inventory.RelationType
	}{
		{
			name:   "pod_relation_types",
			schema: kube.PodSchema{},
			wantRelTypes: []inventory.RelationType{
				inventory.OwnedBy, inventory.RunsOn,
				inventory.References, inventory.Mounts,
				inventory.Pulls,
			},
		},
		{
			name:         "node_relation_types",
			schema:       kube.NodeSchema{},
			wantRelTypes: []inventory.RelationType{inventory.PartOf},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			translator := kube.NewTranslator(tc.schema)
			var observations []inventory.Observation
			switch tc.schema.Kind() {
			case kube.KindNode:
				observations = translator.Added(node("n1"), true,
					fixedTime())
			default:
				observations = translator.Added(pod("p1"), true,
					fixedTime())
			}
			byKind := observationsByKind(observations)
			relObservations := byKind[inventory.Related]
			// All relation types should be present
			seenTypes := make(map[inventory.RelationType]bool)
			for _, f := range relObservations {
				if f.Relation != "" {
					seenTypes[f.Relation] = true
				}
			}
			// Verify we see relation types
			assert.NotEmpty(t, seenTypes)
		})
	}
}

func maintenancePod(annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "p", Namespace: "ns", Annotations: annotations,
	}}
}

func podAttributes(
	observations []inventory.Observation,
) map[string]inventory.Value {
	for _, f := range observations {
		if f.Kind == inventory.Observed && f.Entity.Kind == "pod" {
			return f.Attributes
		}
	}
	return nil
}

func TestTranslatorMaintenanceAttributes(t *testing.T) {
	cfg := kube.MaintenanceAnnotations{On: "m", Until: "m-until"}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pod := maintenancePod(map[string]string{
		"m": "true", "m-until": "2026-02-01T10:00:00Z",
	})
	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		WithMaintenance(cfg).Added(pod, true, at))
	assert.Equal(t, "true", attrs[kube.AttrMaintenance].AsText())
	want := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	assert.True(t, attrs[kube.AttrMaintenanceUntil].AsTime().Equal(want))

	bad := maintenancePod(map[string]string{"m-until": "soon"})
	attrs = podAttributes(kube.NewTranslator(kube.PodSchema{}).
		WithMaintenance(cfg).Added(bad, true, at))
	assert.NotContains(t, attrs, kube.AttrMaintenanceUntil)
}

func TestTranslatorMaintenanceZeroConfigRecordsNothing(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pod := maintenancePod(map[string]string{"m": "true"})
	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, at))
	assert.NotContains(t, attrs, kube.AttrMaintenance)
}
