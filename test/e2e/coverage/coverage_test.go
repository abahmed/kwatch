package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageCatalog(t *testing.T) {
	path := filepath.Join("coverage.yaml")
	catalog, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog.Entries {
		if entry.Status != "covered" {
			continue
		}
		if !strings.HasPrefix(entry.Test, "TestScenario") {
			t.Fatalf("entry %q has invalid test %q", entry.ID, entry.Test)
		}
		if !scenarioTestExists(t, entry.Test) {
			t.Fatalf("entry %q references missing test %q", entry.ID, entry.Test)
		}
	}
}

func scenarioTestExists(t *testing.T, name string) bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "scenarios", "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	needle := "func " + name + "("
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), needle) {
			return true
		}
	}
	return false
}
