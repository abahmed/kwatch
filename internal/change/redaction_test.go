package change

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestDiffRedactsEnvValues(t *testing.T) {
	deployment := func(password string) *appsv1.Deployment {
		d := &appsv1.Deployment{}
		d.Spec.Template.Spec.Containers = []corev1.Container{{
			Name: "app", Image: "app:1",
			Env: []corev1.EnvVar{{Name: "DB_PASSWORD", Value: password}},
		}}
		return d
	}
	result := Diff(deployment("old-secret"), deployment("new-secret"))
	assertNoLeak(t, result, "old-secret", "new-secret")
	if len(result.Fields) == 0 {
		t.Fatal("env change was not recorded")
	}
}

func TestDiffRedactsConfigMapData(t *testing.T) {
	before := &corev1.ConfigMap{Data: map[string]string{"dsn": "pw=one"}}
	after := &corev1.ConfigMap{Data: map[string]string{"dsn": "pw=two"}}
	result := Diff(before, after)
	assertNoLeak(t, result, "pw=one", "pw=two")
}

func TestDiffKeepsNonSensitiveValues(t *testing.T) {
	before := &appsv1.Deployment{}
	before.Spec.Template.Spec.Containers = []corev1.Container{
		{Name: "app", Image: "app:1"},
	}
	after := before.DeepCopy()
	after.Spec.Template.Spec.Containers[0].Image = "app:2"
	result := Diff(before, after)
	if len(result.Fields) != 1 || result.Fields[0].After != "app:2" {
		t.Fatalf("image change = %+v", result.Fields)
	}
}

func assertNoLeak(t *testing.T, result Result, values ...string) {
	t.Helper()
	for _, field := range result.Fields {
		for _, value := range values {
			if strings.Contains(field.Before+field.After, value) {
				t.Fatalf("field %s leaks %q", field.Path, value)
			}
		}
	}
}
