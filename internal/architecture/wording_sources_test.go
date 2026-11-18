package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// confidenceSource is the one file that turns a score into a word.
const confidenceSource = "internal/rootcause/confidence.go"

// thresholdComparison is a score compared with a confidence threshold,
// by any of the names it has had, or a score compared with a number.
var thresholdComparison = regexp.MustCompile(
	`[<>]=?\s*(rootcause\.|explain\.|scorecard\.)?` +
		`(High|Likely|ConfidenceHigh|ConfidenceLikely|HighConfidence|` +
		`LikelyConfidence)\b|\b(High|Likely|ConfidenceHigh|` +
		`ConfidenceLikely|HighConfidence|LikelyConfidence)\s*[<>]|` +
		`\.Score\s*[<>]=?\s*[0-9.]`)

// TestConfidenceIsWordedInOnePlace keeps chat, the dashboard, traces
// and the scorecard on one wording of a cause's certainty: only
// rootcause.LevelOf and rootcause.CertaintyOf compare a score with the
// thresholds.
func TestConfidenceIsWordedInOnePlace(t *testing.T) {
	root := repositoryRoot(t)
	for _, dir := range []string{"internal", "cmd"} {
		for _, name := range goFilesRecursive(t, filepath.Join(root, dir)) {
			rel, _ := filepath.Rel(root, name)
			if filepath.ToSlash(rel) == confidenceSource {
				continue
			}
			contents, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			for i, line := range strings.Split(string(contents), "\n") {
				if thresholdComparison.MatchString(line) {
					t.Errorf("%s:%d compares a score with a confidence "+
						"threshold; use rootcause.CertaintyOf or LevelOf: %s",
						rel, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

// ruleDefiner is the package that names the rows; it alone may spell a
// rule name as a string.
const ruleDefiner = "internal/rootcause/explain/"

// TestRuleNamesAreConstants keeps a row rename from silently changing
// wording: outside explain, a rule or row name is compared, switched on
// or listed only through explain's Rule constants.
func TestRuleNamesAreConstants(t *testing.T) {
	root := repositoryRoot(t)
	for _, dir := range []string{"internal", "cmd"} {
		for _, name := range goFilesRecursive(t, filepath.Join(root, dir)) {
			rel, _ := filepath.Rel(root, name)
			if strings.HasPrefix(filepath.ToSlash(rel), ruleDefiner) {
				continue
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, name, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			for _, lit := range ruleLiterals(file) {
				t.Errorf("%s:%d names a rule as the string %s; use an "+
					"explain.Rule constant", rel,
					fset.Position(lit.Pos()).Line, lit.Value)
			}
		}
	}
}

// ruleLiterals are the non-empty strings file compares with a rule or
// row, switches on one with, passes to a function about rules, or lists
// in a variable of rules or rows.
func ruleLiterals(file *ast.File) []*ast.BasicLit {
	var out []*ast.BasicLit
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				if ruleExpr(n.X) || ruleExpr(n.Y) {
					out = append(out, namedLiterals(n.X, n.Y)...)
				}
			}
		case *ast.SwitchStmt:
			if n.Tag != nil && ruleExpr(n.Tag) {
				for _, stmt := range n.Body.List {
					clause := stmt.(*ast.CaseClause)
					out = append(out, namedLiterals(clause.List...)...)
				}
			}
		case *ast.CallExpr:
			if ruleName(callName(n.Fun)) {
				out = append(out, namedLiterals(n.Args...)...)
			}
		case *ast.ValueSpec:
			out = append(out, listedRules(n)...)
		}
		return true
	})
	return out
}

// listedRules are the strings in a composite value of a variable named
// for rules or rows (provisionerRules, modalRows), keys or elements.
func listedRules(spec *ast.ValueSpec) []*ast.BasicLit {
	var out []*ast.BasicLit
	for i, name := range spec.Names {
		lower := strings.ToLower(name.Name)
		if i >= len(spec.Values) || !(strings.HasSuffix(lower, "rules") ||
			strings.HasSuffix(lower, "rows")) {
			continue
		}
		list, ok := spec.Values[i].(*ast.CompositeLit)
		if !ok || !nameSet(list.Type) {
			continue
		}
		for _, elt := range list.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Key
			}
			out = append(out, namedLiterals(elt)...)
		}
	}
	return out
}

// nameSet is a list or set of names: []string or map[string]bool.
func nameSet(typ ast.Expr) bool {
	switch t := typ.(type) {
	case *ast.ArrayType:
		return identNamed(t.Elt, "string")
	case *ast.MapType:
		return identNamed(t.Key, "string") && identNamed(t.Value, "bool")
	}
	return false
}

func identNamed(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// ruleExpr is an expression that reads a rule or row name.
func ruleExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "rule" || e.Name == "row"
	case *ast.SelectorExpr:
		return e.Sel.Name == "Rule" || e.Sel.Name == "Row"
	}
	return false
}

// callName is the name of the called function or method.
func callName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// ruleName reports a function about a rule, such as causeRuleIs.
func ruleName(name string) bool {
	return strings.Contains(name, "Rule") || strings.HasPrefix(name, "rule")
}

// namedLiterals are the non-empty string literals among exprs.
func namedLiterals(exprs ...ast.Expr) []*ast.BasicLit {
	var out []*ast.BasicLit
	for _, expr := range exprs {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		if value, err := strconv.Unquote(lit.Value); err == nil &&
			value != "" {
			out = append(out, lit)
		}
	}
	return out
}

// TestDashboardScriptsNameNoRule keeps the browser from branching on a
// rule name: the API sends no rule, so a script that names one would
// hold a string a row rename never updates.
func TestDashboardScriptsNameNoRule(t *testing.T) {
	root := repositoryRoot(t)
	values := ruleConstantValues(t, root)
	web := filepath.Join(root, "internal", "dashboard", "web")
	err := filepath.WalkDir(web, func(path string, entry os.DirEntry,
		err error,
	) error {
		if err != nil || entry.IsDir() ||
			!strings.HasSuffix(path, ".js") &&
				!strings.HasSuffix(path, ".html") {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range values {
			// A one-word name ("self", "rollout") is also a plain
			// word the pages use; only a hyphenated name is a rule's.
			if !strings.Contains(value, "-") {
				continue
			}
			for _, quote := range []string{`"`, `'`} {
				if strings.Contains(string(contents), quote+value+quote) {
					t.Errorf("%s names rule %q", path, value)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", web, err)
	}
}

// ruleConstantValues are the values of explain's Rule constants.
func ruleConstantValues(t *testing.T, root string) []string {
	t.Helper()
	name := filepath.Join(root, ruleDefiner, "rule_names.go")
	var out []string
	ast.Inspect(parseFile(t, name), func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, ident := range spec.Names {
			if !strings.HasPrefix(ident.Name, "Rule") ||
				i >= len(spec.Values) {
				continue
			}
			for _, lit := range namedLiterals(spec.Values[i]) {
				value, _ := strconv.Unquote(lit.Value)
				out = append(out, value)
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatalf("%s declares no Rule constants", name)
	}
	return out
}
