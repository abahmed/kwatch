//go:build e2e

package harness

import "testing"

func TestCheckContext(t *testing.T) {
	tests := []struct {
		name  string
		allow bool
		ok    bool
	}{
		{"kind-kwatch", false, true},
		{"kind-", false, true},
		{"prod-cluster", false, false},
		{"arn:aws:eks:eu-west-1:1:cluster/x", false, false},
		{"", false, false},
		{"prod-cluster", true, true},
	}
	for _, tc := range tests {
		err := checkContext(tc.name, tc.allow)
		if (err == nil) != tc.ok {
			t.Errorf("checkContext(%q, %v) = %v", tc.name, tc.allow, err)
		}
	}
}

func TestConfigFromEnvCheckedRejectsNonKindContext(t *testing.T) {
	t.Setenv("KUBE_CONTEXT", "production")
	t.Setenv(allowAnyContextEnv, "")
	if _, err := ConfigFromEnvChecked(); err == nil {
		t.Fatal("a non-kind context must be refused")
	}
	t.Setenv("KUBE_CONTEXT", "kind-kwatch")
	if _, err := ConfigFromEnvChecked(); err != nil {
		t.Fatalf("kind context refused: %v", err)
	}
	t.Setenv("KUBE_CONTEXT", "production")
	t.Setenv(allowAnyContextEnv, "true")
	if _, err := ConfigFromEnvChecked(); err != nil {
		t.Fatalf("opt-in refused: %v", err)
	}
}
