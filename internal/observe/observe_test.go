package observe

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestPodObservationFillsIdentityAndOwner(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api", UID: "uid-1",
		GenerateName: "api-", Labels: map[string]string{"app": "api"},
		Annotations: map[string]string{PodLineageAnnotation: "lineage"},
	}, Spec: corev1.PodSpec{NodeName: "node-a"}}
	obs := PodOwnedBy(
		pod, "app", constant.ReasonContainerMemoryHigh,
		SelfOwner("Deployment", "apps", "api"),
	)
	if obs.Subject.Kind != "pod" || obs.Subject.Name != "api" ||
		obs.Subject.Namespace != "apps" || obs.Owner.Kind != "Deployment" ||
		obs.Pod.UID != "uid-1" || obs.Pod.LineageID != "lineage" ||
		obs.NodeName != "node-a" || obs.Labels["app"] != "api" {
		t.Fatalf("unexpected pod observation: %+v", obs)
	}
}

func TestObjectAndClusterObservationBuilders(t *testing.T) {
	obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api", Labels: map[string]string{"tier": "web"},
	}}
	object := Object("service", obj, "ServiceFailure")
	if object.Owner != object.Subject || object.Labels["tier"] != "web" {
		t.Fatalf("unexpected object observation: %+v", object)
	}
	got := VolumeUsage("apps", "data", "api", "VolumeHigh")
	if got.Owner.Name != "data" || got.Subject.Name != "api" {
		t.Fatalf("unexpected volume observation: %+v", got)
	}
	if got := Namespace(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "apps",
	}}, "NamespaceFailure"); got.Owner.Name != "apps" ||
		got.Subject.Namespace != "apps" {
		t.Fatalf("unexpected namespace observation: %+v", got)
	}
	if got := NodeNamed("node-a", "NodeFailure"); got.NodeName != "node-a" ||
		got.Subject.Kind != "node" {
		t.Fatalf("unexpected node observation: %+v", got)
	}
	got = Synthetic("activeprobe", "api", "ProbeFailure")
	if got.Owner.Name != "api" || got.Subject.Kind != "activeprobe" {
		t.Fatalf("unexpected synthetic observation: %+v", got)
	}
}

func TestNilObservationBuildersRemainSafe(t *testing.T) {
	if got := Pod(nil, ".", "PodFailure", nil); got.Owner.Name != "" {
		t.Fatalf("nil pod owner = %+v", got.Owner)
	}
	if got := Object("service", nil, "ServiceFailure"); got.Subject.Name != "" {
		t.Fatalf("nil object = %+v", got.Subject)
	}
	if got := Node(nil, "NodeFailure"); got.Subject.Name != "" {
		t.Fatalf("nil node = %+v", got.Subject)
	}
}
