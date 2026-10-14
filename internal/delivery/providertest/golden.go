package providertest

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateAllEnv is the deliberate opt-in to regenerate every contract
// golden at once.
const updateAllEnv = "KWATCH_UPDATE_ALL"

var update = flag.Bool("update", false,
	"rewrite provider contract goldens (needs a scoped -run)")

// updateScopeError says why a -update run with this -run pattern may not
// proceed, or returns nil. It follows the scenarios guard: the pattern
// must name a subtest that is not a match-everything wildcard.
func updateScopeError(run, updateAll string) error {
	if updateAll == "1" {
		return nil
	}
	parts := strings.Split(run, "/")
	if len(parts) >= 2 {
		sub := strings.Trim(parts[1], "^$")
		if sub != "" && sub != ".*" && sub != ".+" && sub != "." {
			return nil
		}
	}
	return fmt.Errorf("-update needs a -run filter that picks specific "+
		"cases, got %q; use for example -run "+
		"'TestContract/page-open$' -update, or set %s=1 for a "+
		"deliberate full regeneration", run, updateAllEnv)
}

func requireScopedUpdate(t *testing.T) {
	t.Helper()
	run := ""
	if f := flag.Lookup("test.run"); f != nil {
		run = f.Value.String()
	}
	if err := updateScopeError(run, os.Getenv(updateAllEnv)); err != nil {
		t.Fatal(err)
	}
}

// CompareGolden checks got against the committed golden file at path,
// or rewrites it under a scoped -update.
func CompareGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		requireScopedUpdate(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with a scoped -update): %v",
			path, err)
	}
	if string(want) != got {
		t.Fatalf("request differs from %s\n--- want\n%s\n--- got\n%s",
			path, want, got)
	}
}
