package coverage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// reasonGapsFile lists reasons that no real-cluster scenario exercises yet.
// Every emitted reason must be covered or listed, so a new reason cannot
// ship without a decision about its regression coverage.
const reasonGapsFile = "reason-gaps.txt"

func TestReasonParityEveryReasonIsCoveredOrListed(t *testing.T) {
	reasons := constantReasons(t)
	used := scenarioStrings(t)
	gaps := readGaps(t)
	var missing, stale []string
	for reason := range reasons {
		if !used[reason] && !gaps[reason] {
			missing = append(missing, reason)
		}
	}
	for gap := range gaps {
		if used[gap] || !reasons[gap] {
			stale = append(stale, gap)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("reasons without a scenario or a %s entry: %s",
			reasonGapsFile, strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		t.Errorf("%s lists covered or unknown reasons; remove: %s",
			reasonGapsFile, strings.Join(stale, ", "))
	}
}

func constantReasons(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "..", "internal", "constant",
		"reason.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "Reason") ||
				i >= len(spec.Values) {
				continue
			}
			if lit, ok := spec.Values[i].(*ast.BasicLit); ok {
				if value, err := strconv.Unquote(lit.Value); err == nil {
					out[value] = true
				}
			}
		}
		return true
	})
	return out
}

func scenarioStrings(t *testing.T) map[string]bool {
	t.Helper()
	out := make(map[string]bool)
	dir := filepath.Join("..", "scenarios")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(
			token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0,
		)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if lit, ok := node.(*ast.BasicLit); ok &&
				lit.Kind == token.STRING {
				if value, err := strconv.Unquote(lit.Value); err == nil {
					out[value] = true
				}
			}
			return true
		})
	}
	return out
}

func readGaps(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(reasonGapsFile)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			out[line] = true
		}
	}
	return out
}
