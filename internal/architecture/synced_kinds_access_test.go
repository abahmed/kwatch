package architecture

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

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kindConstants reads the string constants of the kube package:
// "KindPriorityClass" -> "priorityclass".
func kindConstants(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../inventory/kube/*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
				return true
			}
			lit, ok := spec.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, _ := strconv.Unquote(lit.Value)
			out[spec.Names[0].Name] = value
			return true
		})
	}
	return out
}

// kubeKindName returns "KindX" for the expression kube.KindX.
func kubeKindName(e ast.Expr) (string, bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "kube" ||
		!strings.HasPrefix(sel.Sel.Name, "Kind") {
		return "", false
	}
	return sel.Sel.Name, true
}

// syncedKinds lists the kube.Kind constants the detectors ask whether a
// source synced: the argument of a Synced call, the kinds of a kind list
// they loop over, and the keys of the classReason table. Each of them
// needs a read grant, or the detector never has data to conclude from.
func syncedKinds(t *testing.T) []string {
	t.Helper()
	dir := "../detection/detectors"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(),
			filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			collectSynced(n, found)
			return true
		})
	}
	out := make([]string, 0, len(found))
	for kind := range found {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

func collectSynced(n ast.Node, found map[string]bool) {
	switch v := n.(type) {
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "Synced" && len(v.Args) == 1 {
			if kind, ok := kubeKindName(v.Args[0]); ok {
				found[kind] = true
			}
		}
	case *ast.RangeStmt:
		if lit, ok := v.X.(*ast.CompositeLit); ok {
			addKinds(lit.Elts, found)
		}
	case *ast.ValueSpec:
		if len(v.Names) == 1 && v.Names[0].Name == "classReason" &&
			len(v.Values) == 1 {
			if lit, ok := v.Values[0].(*ast.CompositeLit); ok {
				for _, elt := range lit.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						addKinds([]ast.Expr{kv.Key}, found)
					}
				}
			}
		}
	}
}

func addKinds(exprs []ast.Expr, found map[string]bool) {
	for _, e := range exprs {
		if kind, ok := kubeKindName(e); ok {
			found[kind] = true
		}
	}
}

// resourceNames is every resource name SourceAccess lists for list.
func resourceNames() map[string]bool {
	names := map[string]bool{}
	for _, a := range kube.SourceAccess() {
		if a.NonResourceURL == "" && a.Verb == "list" {
			names[a.Resource.Name] = true
		}
	}
	return names
}

// pluralCandidates are the resource names an entity kind may have.
func pluralCandidates(kind string) []string {
	return []string{kind + "s", kind + "es",
		strings.TrimSuffix(kind, "y") + "ies"}
}

func TestSyncedKindsAreInSourceAccess(t *testing.T) {
	consts := kindConstants(t)
	resources := resourceNames()
	kinds := syncedKinds(t)
	if len(kinds) < 5 {
		t.Fatalf("found only %v: the scan is broken", kinds)
	}
	for _, constName := range kinds {
		value, ok := consts[constName]
		if !ok {
			t.Errorf("kube.%s is not a kind constant", constName)
			continue
		}
		granted := false
		for _, candidate := range pluralCandidates(value) {
			granted = granted || resources[candidate]
		}
		if !granted {
			t.Errorf("detectors ask whether kube.%s is synced, but "+
				"kube.SourceAccess() lists no read access for it: add "+
				"it to the informer registrations or to "+
				"auditedDynamicResources so the least-privilege chart "+
				"rules grant it", constName)
		}
	}
}
