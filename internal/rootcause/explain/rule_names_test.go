package explain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// ruleConstants reads the exported Rule constants of rule_names.go, so
// a constant added there is checked without listing it again.
func ruleConstants(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "rule_names.go",
		nil, 0)
	if err != nil {
		t.Fatalf("parse rule_names.go: %v", err)
	}
	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Values) != len(spec.Names) {
			return true
		}
		for i, name := range spec.Names {
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || !strings.HasPrefix(name.Name, "Rule") {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: %v", name.Name, err)
			}
			out[name.Name] = value
		}
		return true
	})
	return out
}

// TestRuleConstantsNameOneRowEach keeps the names other packages branch
// on true: renaming a row without its constant fails here instead of
// silently changing a message's wording. Each constant names a row of
// the table (or the self row), and no two constants name the same one.
// A row built once per link type (finalizer-handler-stopped) is still
// one row.
func TestRuleConstantsNameOneRowEach(t *testing.T) {
	constants := ruleConstants(t)
	if len(constants) == 0 {
		t.Fatal("rule_names.go declares no Rule constants")
	}
	rows := map[string]bool{selfRow.Name: true}
	for _, row := range Table() {
		rows[row.Name] = true
	}
	named := map[string]string{}
	for name, value := range constants {
		if !rows[value] {
			t.Errorf("%s = %q names no row of the table", name, value)
		}
		if other, ok := named[value]; ok {
			t.Errorf("%s and %s both name row %q", name, other, value)
		}
		named[value] = name
	}
}
