package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const knative = "serving.knative.dev"

func TestGroupForAPIVersionMapsBuiltinGroupsToCore(t *testing.T) {
	tests := []struct{ apiVersion, want string }{
		{"v1", ""},
		{"apps/v1", ""},
		{"networking.k8s.io/v1", ""},
		{"apiregistration.k8s.io/v1", ""},
		{"", ""},
		{"a/b/c", ""},
		{"gateway.networking.k8s.io/v1", "gateway.networking.k8s.io"},
		{"networking.istio.io/v1", "networking.istio.io"},
		{"serving.knative.dev/v1", knative},
	}
	for _, tt := range tests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			assert.Equal(t, tt.want, kube.GroupForAPIVersion(tt.apiVersion))
		})
	}
}

func TestOwnerReferenceResolvesCollidingKindByGroup(t *testing.T) {
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "p", Namespace: "ns",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: knative + "/v1", Kind: "Service", Name: "web",
			Controller: &controller,
		}},
	}}
	d, ok := kube.PodSchema{}.Describe(pod)
	require.True(t, ok)
	owners := d.Relations[inventory.OwnedBy]
	assert.Contains(t, owners,
		inventory.NewEntityID(knative, kube.KindService, "ns", "web"))
	assert.NotContains(t, owners,
		inventory.CoreID(kube.KindService, "ns", "web"))
}

func TestUnstructuredSchemaSeparatesGroupsOfOneKind(t *testing.T) {
	gatewayAPI := kube.NewUnstructuredSchema(
		"gateway.networking.k8s.io", "Gateway")
	istio := kube.NewUnstructuredSchema("networking.istio.io", "Gateway")
	gw := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.istio.io/v1", "kind": "Gateway",
		"metadata": map[string]any{"name": "gw", "namespace": "infra"},
	}}
	_, ok := gatewayAPI.Describe(gw)
	assert.False(t, ok)
	d, ok := istio.Describe(gw)
	require.True(t, ok)
	assert.Equal(t, inventory.NewEntityID("networking.istio.io", "gateway",
		"infra", "gw"), d.ID)
	assert.Equal(t, "gateway.networking.istio.io/infra/gw", d.ID.String())
}

func TestEventNoteUsesInvolvedObjectGroup(t *testing.T) {
	ev := warningEvent("Service", "")
	ev.InvolvedObject.APIVersion = knative + "/v1"
	o, ok := kube.EventNote(ev, fixedTime())
	require.True(t, ok)
	assert.Equal(t, inventory.NewEntityID(knative, kube.KindService,
		testNamespace, "p"), o.Entity)
}

func TestHPAScaleTargetUsesTargetGroup(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "h", Namespace: "ns"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "argoproj.io/v1alpha1", Kind: "Rollout",
				Name: "api",
			},
		},
	}
	d, ok := kube.HPASchema{}.Describe(hpa)
	require.True(t, ok)
	assert.Equal(t, []inventory.EntityID{
		inventory.NewEntityID("argoproj.io", "rollout", "ns", "api"),
	}, d.Relations[inventory.Scales])
}
