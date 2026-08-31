package change

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiffIgnoresStatusAndRedactsSecret(t *testing.T) {
	oldPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:v1"}}}}
	newPod := oldPod.DeepCopy()
	newPod.Spec.Containers[0].Image = "api:v2"
	newPod.Status.Phase = corev1.PodRunning
	result := Diff(oldPod, newPod)
	if len(result.Fields) != 1 || result.Fields[0].Path != "spec.containers.name=api.image" ||
		result.Fields[0].Before != "api:v1" || result.Fields[0].After != "api:v2" {
		t.Fatalf("unexpected pod diff: %#v", result.Fields)
	}

	oldSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "config"}, StringData: map[string]string{"token": "old"}}
	newSecret := oldSecret.DeepCopy()
	newSecret.StringData["token"] = "new"
	secretDiff := Diff(oldSecret, newSecret)
	if len(secretDiff.Fields) != 0 || strings.Contains(secretDiff.BeforeHash, "old") || strings.Contains(secretDiff.AfterHash, "new") {
		t.Fatalf("secret value leaked in diff: %#v", secretDiff.Fields)
	}
}
