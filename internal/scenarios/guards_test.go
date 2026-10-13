package scenarios

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// updateAllEnv is the deliberate opt-in to regenerate every fixture and
// golden at once, for integration passes.
const updateAllEnv = "KWATCH_UPDATE_ALL"

// updateScopeError says why a -update run with this -run pattern may not
// proceed, or returns nil. -update rewrites committed files, so it must
// name the scenarios it rewrites: the pattern needs a subtest element
// (Test/name) that is not a match-everything wildcard.
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
		"scenarios, got %q; use for example -run "+
		"'TestScenarioMessagesGolden/bad-rollout$' -update, or set %s=1 "+
		"for a deliberate full regeneration", run, updateAllEnv)
}

// requireScopedUpdate stops a test that is about to rewrite committed files
// when -update was not scoped to specific scenarios.
func requireScopedUpdate(t *testing.T) {
	t.Helper()
	if !*update {
		return
	}
	run := ""
	if f := flag.Lookup("test.run"); f != nil {
		run = f.Value.String()
	}
	if err := updateScopeError(run, os.Getenv(updateAllEnv)); err != nil {
		t.Fatal(err)
	}
}

// orphanFiles lists the files in dir that end in one of suffixes but whose
// scenario name is not in want, and the wanted files that are missing.
func orphanFiles(dir string, want []string, suffixes ...string) []string {
	have := map[string]bool{}
	var problems []string
	for _, suffix := range suffixes {
		files, _ := filepath.Glob(filepath.Join(dir, "*"+suffix))
		for _, f := range files {
			have[filepath.Base(f)] = true
		}
	}
	registered := map[string]bool{}
	for _, name := range want {
		for _, suffix := range suffixes {
			file := name + suffix
			registered[file] = true
			if !have[file] {
				problems = append(problems, "missing "+file)
			}
		}
	}
	for file := range have {
		if !registered[file] {
			problems = append(problems, "orphan "+file)
		}
	}
	sort.Strings(problems)
	return problems
}

func scenarioNames(scenarios []scenario) []string {
	names := make([]string, 0, len(scenarios))
	for _, s := range scenarios {
		names = append(names, s.expect.Name)
	}
	return names
}

// TestScenarioFilesMatchRegistry fails on a committed log, label or message
// golden with no registered scenario, and on a registered scenario that is
// missing a file.
func TestScenarioFilesMatchRegistry(t *testing.T) {
	t.Parallel()
	labelled := scenarioNames(library())
	cases := []struct {
		dir      string
		names    []string
		suffixes []string
	}{
		{labelledDir, labelled, []string{".jsonl", ".expect.json"}},
		{heldoutDir, scenarioNames(heldoutLibrary()),
			[]string{".jsonl", ".expect.json"}},
		{filepath.Join("testdata", "messages"), labelled,
			[]string{".golden"}},
	}
	for _, c := range cases {
		for _, p := range orphanFiles(c.dir, c.names, c.suffixes...) {
			t.Errorf("%s: %s", c.dir, p)
		}
	}
}

func TestOrphanFilesReportsBothDirections(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a.golden", "stray.golden"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := strings.Join(orphanFiles(dir, []string{"a", "b"}, ".golden"), ",")
	if want := "missing b.golden,orphan stray.golden"; got != want {
		t.Errorf("problems = %q, want %q", got, want)
	}
}

func TestUpdateScopeRefusesUnscopedRuns(t *testing.T) {
	t.Parallel()
	refused := []string{
		"", "TestScenarioFixtures", "TestScenarioMessagesGolden/",
		"TestScenarioMessagesGolden/.*", "TestScenarioFixtures/^$",
		"Test/.*$", ".*",
	}
	for _, run := range refused {
		if updateScopeError(run, "") == nil {
			t.Errorf("-run %q was allowed with -update", run)
		}
	}
	allowed := []string{
		"TestScenarioMessagesGolden/bad-rollout$",
		"TestScenarioFixtures/^bad-rollout$",
	}
	for _, run := range allowed {
		if err := updateScopeError(run, ""); err != nil {
			t.Errorf("-run %q refused: %v", run, err)
		}
	}
	if err := updateScopeError("", "1"); err != nil {
		t.Errorf("%s=1 refused: %v", updateAllEnv, err)
	}
	if updateScopeError("", "0") == nil {
		t.Errorf("%s=0 must not opt in", updateAllEnv)
	}
}
