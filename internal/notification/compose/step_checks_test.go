package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// TestStepChecksCoverReadOnlySteps reads every notification.Step literal
// compose writes and fails when one with a command and without
// Mutating: true has no covering check: Pro would show a read-only
// command kwatch never runs for the reader.
func TestStepChecksCoverReadOnlySteps(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		for _, lead := range readOnlyCommands(t, name) {
			found++
			if stepCheck(lead) == "" {
				t.Errorf("%s: read-only step %q has no covering check",
					name, lead)
			}
		}
	}
	if found < 20 {
		t.Fatalf("found %d read-only commands; the scan is broken", found)
	}
}

// readOnlyCommands returns the leading literal of the Command of every
// Step literal in file that is not marked Mutating: true.
func readOnlyCommands(t *testing.T, file string) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isStepLiteral(lit) {
			return true
		}
		command, mutating := stepFields(lit)
		if command != nil && !mutating {
			out = append(out, leadingLiteral(t, command))
		}
		return true
	})
	return out
}

// isStepLiteral reports a notification.Step literal, or an element of a
// []notification.Step literal (which has no type of its own).
func isStepLiteral(lit *ast.CompositeLit) bool {
	if sel, ok := lit.Type.(*ast.SelectorExpr); ok {
		return sel.Sel.Name == "Step"
	}
	if lit.Type != nil {
		return false
	}
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Command" {
				return true
			}
		}
	}
	return false
}

func stepFields(lit *ast.CompositeLit) (ast.Expr, bool) {
	var command ast.Expr
	mutating := false
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch kv.Key.(*ast.Ident).Name {
		case "Command":
			command = kv.Value
		case "Mutating":
			id, ok := kv.Value.(*ast.Ident)
			mutating = ok && id.Name == "true"
		}
	}
	return command, mutating
}

// leadingLiteral is the leftmost string literal of a + chain.
func leadingLiteral(t *testing.T, e ast.Expr) string {
	t.Helper()
	for {
		switch v := e.(type) {
		case *ast.BinaryExpr:
			e = v.X
		case *ast.BasicLit:
			s, err := strconv.Unquote(v.Value)
			if err != nil {
				t.Fatal(err)
			}
			return s
		default:
			t.Fatalf("a Command must start with a string literal: %T", e)
			return ""
		}
	}
}

func TestStepChecksNameSharedChecks(t *testing.T) {
	for _, c := range stepChecks {
		if !slices.Contains(incident.StepChecks, c.check) {
			t.Errorf("%q maps to %q, which incident.StepChecks lacks",
				c.prefix, c.check)
		}
	}
}

func TestGatheredStepsKeepsFixAndUncoveredSteps(t *testing.T) {
	steps := []notification.Step{
		{Text: "read", Command: "kubectl logs api -n shop --previous"},
		{Text: "top", Command: "kubectl top pod api -n shop --containers"},
		{Text: "undo", Command: "kubectl rollout undo deployment/api",
			Mutating: true},
		{Text: "advice"},
	}
	got := gatheredSteps(steps, map[string]bool{incident.CheckLogs: true})
	want := []string{"top", "undo", "advice"}
	var texts []string
	for _, s := range got {
		texts = append(texts, s.Text)
	}
	if !slices.Equal(texts, want) {
		t.Fatalf("steps = %v, want %v", texts, want)
	}
	if kept := gatheredSteps(steps, nil); len(kept) != len(steps) {
		t.Fatal("without a report every step must stay")
	}
}
