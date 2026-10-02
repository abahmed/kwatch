package coverage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

var detectorsDir = filepath.Join(
	"..", "..", "..", "internal", "detection", "detectors")

// topModes are the failure modes every operator expects kwatch to catch;
// each must map to an end-to-end scenario, even if only as pending.
var topModes = []string{
	"CrashLoop", "ImagePull", "OOMKilled", "MemoryPressure", "NotReady",
	"RolloutStuck", "Missing.ConfigMap", "Unschedulable",
	"ClaimFailed", "Unavailable.CoreDNS",
}

// maxUncoveredModes is a ratchet: modes without a detector unit test are
// listed in the failure, and the count may only go down.
const maxUncoveredModes = 30

func TestFailureModesEveryModeHasUnitTest(t *testing.T) {
	referenced := testReferences(t)
	reasonNames := reasonConstants(t)
	var uncovered []string
	for _, entry := range detection.Modes() {
		if !modeReferenced(entry, referenced, reasonNames) {
			uncovered = append(uncovered, string(entry.Mode))
		}
	}
	sort.Strings(uncovered)
	if len(uncovered) > maxUncoveredModes {
		t.Errorf("modes without a detector unit test (%d, max %d): %s",
			len(uncovered), maxUncoveredModes,
			strings.Join(uncovered, ", "))
	} else if len(uncovered) > 0 {
		t.Logf("modes without a detector unit test (%d): %s",
			len(uncovered), strings.Join(uncovered, ", "))
	}
}

func modeReferenced(
	entry detection.ModeEntry, referenced map[string]bool,
	reasonNames map[string]string,
) bool {
	if referenced[string(entry.Mode)] {
		return true
	}
	for _, reason := range entry.Reasons {
		if referenced[reason] || referenced[reasonNames[reason]] {
			return true
		}
	}
	return false
}

func TestFailureModesTopModesMapToScenarios(t *testing.T) {
	catalog, err := Load("coverage.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mapped := make(map[string]bool)
	for _, entry := range catalog.Entries {
		for _, mode := range entry.Modes {
			mapped[mode] = true
		}
	}
	known := make(map[string]bool)
	for _, entry := range detection.Modes() {
		known[string(entry.Mode)] = true
	}
	for _, mode := range topModes {
		if !known[mode] {
			t.Errorf("top mode %q is not in the detection mode table", mode)
		}
		if !mapped[mode] {
			t.Errorf("top mode %q has no coverage.yaml entry", mode)
		}
	}
	for mode := range mapped {
		if !known[mode] {
			t.Errorf("coverage.yaml lists unknown mode %q", mode)
		}
	}
}

// testReferences collects every string literal and reasons.<Name>
// selector used by the detector unit tests.
func testReferences(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(detectorsDir, "*_test.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no detector tests found: %v", err)
	}
	out := make(map[string]bool)
	for _, path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.BasicLit:
				if v, err := strconv.Unquote(n.Value); err == nil &&
					n.Kind == token.STRING {
					out[v] = true
				}
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && id.Name == "reasons" {
					out[n.Sel.Name] = true
				}
			}
			return true
		})
	}
	return out
}

// reasonConstants maps each reason value to its constant name.
func reasonConstants(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "internal", "detection",
		"reasons", "reasons.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string)
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			if v, err := strconv.Unquote(lit.Value); err == nil {
				out[v] = name.Name
			}
		}
		return true
	})
	return out
}
