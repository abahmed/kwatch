package issue

import (
	"strings"
	"testing"
)

func TestValidateRejectsSecretsAndCommands(t *testing.T) {
	for _, payload := range []string{
		"token: value",
		"command: kubectl delete pod",
		"privileged: true",
	} {
		if err := Validate(Blocks{Resources: payload}); err == nil {
			t.Fatalf("expected unsafe payload %q to be rejected", payload)
		}
	}
}

func TestValidateAllowsMinimalResources(t *testing.T) {
	blocks := Blocks{Resources: "kind: Pod\nmetadata:\n  name: test"}
	if err := Validate(blocks); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeResourcesRewritesNamespaceAndImage(t *testing.T) {
	resources := `apiVersion: v1
kind: Pod
metadata:
  name: reproduction
  namespace: reported
spec:
  containers:
  - name: app
    image: private.example/app:v1
`
	got, err := SanitizeResources(
		resources, "scenario", "kwatch-e2e-workload:test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "reported") ||
		strings.Contains(got, "private.example") {
		t.Fatalf("sanitized resources retained unsafe values: %s", got)
	}
	if !strings.Contains(got, "namespace: scenario") ||
		!strings.Contains(got, "image: kwatch-e2e-workload:test") {
		t.Fatalf("sanitized resources lost replacements: %s", got)
	}
}

func TestSanitizeResourcesRejectsUnsafeKind(t *testing.T) {
	_, err := SanitizeResources(
		"kind: ClusterRole\nmetadata:\n  name: bad\n",
		"scenario", "kwatch-e2e-workload:test",
	)
	if err == nil {
		t.Fatal("expected unsafe kind to be rejected")
	}
}

func TestSanitizeResourcesAddsNamespaceWhenMissing(t *testing.T) {
	resources := "kind: Pod\nmetadata:\n  name: reproduction\n"
	got, err := SanitizeResources(
		resources, "scenario", "kwatch-e2e-workload:test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "namespace: scenario") {
		t.Fatalf("sanitized resource has no namespace: %s", got)
	}
}
