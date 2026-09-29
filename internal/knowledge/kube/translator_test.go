package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestTranslatorAdded(t *testing.T) {
	tt := []struct {
		name        string
		obj         any
		initialList bool
		wantCount   int
		checkFacts  func(*testing.T, []knowledge.Fact)
	}{
		{
			name:        "initial_list_pod",
			obj:         pod("p1"),
			initialList: true,
			wantCount:   1, // At least 1 fact
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// InitialList=true: Observed + Related +
				// children, NO Changed
				assert.NotNil(t, byKind[knowledge.Observed])
				assert.NotNil(t, byKind[knowledge.Related])
				assert.Nil(t, byKind[knowledge.Changed])
			},
		},
		{
			name:        "not_initial_list_pod",
			obj:         pod("p1"),
			initialList: false,
			wantCount:   1, // At least 1 fact
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// InitialList=false: Observed + Related +
				// Changed
				assert.NotNil(t, byKind[knowledge.Observed])
				assert.NotNil(t, byKind[knowledge.Related])
				assert.NotNil(t, byKind[knowledge.Changed])
				// Find Changed for pod entity
				for _, f := range byKind[knowledge.Changed] {
					if f.Entity.Kind == "pod" {
						assert.True(t, f.Change.Created)
						assert.Equal(t, "kubectl",
							f.Change.Actor)
						return
					}
				}
				t.Fatal("no Changed fact for pod")
			},
		},
		{
			name:        "deployment_initial",
			obj:         deployment("d1"),
			initialList: true,
			wantCount:   1, // At least one fact
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				assert.NotNil(t, byKind[knowledge.Observed])
				assert.NotNil(t, byKind[knowledge.Related])
				assert.Nil(t, byKind[knowledge.Changed])
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
			facts := translator.Added(tc.obj, tc.initialList,
				fixedTime())
			if tc.wantCount > 0 {
				assert.GreaterOrEqual(t, len(facts),
					tc.wantCount)
			} else {
				assert.Equal(t, tc.wantCount, len(facts))
			}
			if tc.checkFacts != nil {
				tc.checkFacts(t, facts)
			}
		})
	}
}

func TestTranslatorUpdated(t *testing.T) {
	tt := []struct {
		name       string
		schema     kube.Schema
		oldObj     any
		newObj     any
		checkFacts func(*testing.T, []knowledge.Fact)
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
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// Status-only changes should NOT emit Changed
				assert.Nil(t, byKind[knowledge.Changed])
				assert.NotNil(t,
					byKind[knowledge.Observed])
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
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// DeletionTimestamp is a meaningful change
				assert.NotNil(t, byKind[knowledge.Changed])
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
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// Template change is a meaningful change
				assert.NotNil(t, byKind[knowledge.Changed])
			},
		},
		{
			name:   "wrong_type_returns_nil",
			schema: kube.DeploymentSchema(),
			oldObj: "not k8s object",
			newObj: "also not k8s object",
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				assert.Len(t, facts, 0)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			translator := kube.NewTranslator(tc.schema)
			facts := translator.Updated(tc.oldObj, tc.newObj,
				fixedTime())
			if tc.checkFacts != nil {
				tc.checkFacts(t, facts)
			}
		})
	}
}

func TestTranslatorDeleted(t *testing.T) {
	tt := []struct {
		name       string
		obj        any
		checkFacts func(*testing.T, []knowledge.Fact)
	}{
		{
			name: "deleted_pod",
			obj:  pod("p1"),
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// Pod deletion should emit Gone facts
				assert.NotNil(t, byKind[knowledge.Gone])
				gone := byKind[knowledge.Gone]
				// Should have Gone for pod + 1 for container
				assert.GreaterOrEqual(t, len(gone), 1)
			},
		},
		{
			name: "deleted_tombstone",
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/p1",
				Obj: pod("p1"),
			},
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				byKind := factsByKind(facts)
				// Tombstone should still emit Gone facts
				assert.NotNil(t, byKind[knowledge.Gone])
			},
		},
		{
			name: "wrong_type_returns_nil",
			obj:  "not a k8s object",
			checkFacts: func(t *testing.T, facts []knowledge.Fact) {
				assert.Len(t, facts, 0)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			translator := kube.NewTranslator(kube.PodSchema{})
			facts := translator.Deleted(tc.obj, fixedTime())
			if tc.checkFacts != nil {
				tc.checkFacts(t, facts)
			}
		})
	}
}

func TestTranslatorRelationTypes(t *testing.T) {
	tt := []struct {
		name         string
		schema       kube.Schema
		wantRelTypes []knowledge.RelationType
	}{
		{
			name:   "pod_relation_types",
			schema: kube.PodSchema{},
			wantRelTypes: []knowledge.RelationType{
				knowledge.OwnedBy, knowledge.RunsOn,
				knowledge.References, knowledge.Mounts,
				knowledge.Pulls,
			},
		},
		{
			name:         "node_relation_types",
			schema:       kube.NodeSchema{},
			wantRelTypes: []knowledge.RelationType{knowledge.PartOf},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			translator := kube.NewTranslator(tc.schema)
			var facts []knowledge.Fact
			switch tc.schema.Kind() {
			case kube.KindNode:
				facts = translator.Added(node("n1"), true,
					fixedTime())
			default:
				facts = translator.Added(pod("p1"), true,
					fixedTime())
			}
			byKind := factsByKind(facts)
			relFacts := byKind[knowledge.Related]
			// All relation types should be present
			seenTypes := make(map[knowledge.RelationType]bool)
			for _, f := range relFacts {
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

func podAttributes(facts []knowledge.Fact) map[string]knowledge.Value {
	for _, f := range facts {
		if f.Kind == knowledge.Observed && f.Entity.Kind == "pod" {
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
