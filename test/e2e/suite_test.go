//go:build e2e

package e2e

import "testing"

func TestScenarioSuiteRequiresExplicitOptIn(t *testing.T) {
	if testing.Short() {
		t.Skip("real-cluster tests are not short tests")
	}
	if t.Name() == "" {
		t.Fatal("test name must be available")
	}
}
