package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// detectorUsage records, per reason value, the detector source files that
// reference it and the severities they assign.
type detectorUsage struct {
	files      map[string]map[string]bool
	severities map[string]map[string]bool
}

func (u detectorUsage) detectors(reasonValues []string) []string {
	set := make(map[string]bool)
	for _, r := range reasonValues {
		for f := range u.files[r] {
			set[f] = true
		}
	}
	return sortedKeys(set)
}

func (u detectorUsage) health(reasonValues []string) string {
	set := make(map[string]bool)
	for _, r := range reasonValues {
		for s := range u.severities[r] {
			set[s] = true
		}
	}
	switch {
	case len(set) == 0:
		return "varies"
	case len(set) > 1:
		return "varies"
	case set["Critical"]:
		return "failing"
	default:
		return "degraded"
	}
}

func scanDetectors(root string) (detectorUsage, error) {
	names, err := reasonNames(root)
	if err != nil {
		return detectorUsage{}, err
	}
	usage := detectorUsage{
		files:      make(map[string]map[string]bool),
		severities: make(map[string]map[string]bool),
	}
	dir := filepath.Join(root, "internal", "detection", "detectors")
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return usage, err
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return usage, err
		}
		base := strings.TrimSuffix(filepath.Base(path), ".go")
		scanFile(file, base, names, usage)
	}
	return usage, nil
}

func scanFile(
	file *ast.File, base string, names map[string]string, u detectorUsage,
) {
	reads := readOnlyReferences(file)
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			if v, ok := reasonValue(n, names); ok && !reads[n] {
				add(u.files, v, base)
			}
		case *ast.CompositeLit:
			recordLiteral(n, names, u)
		case *ast.KeyValueExpr:
			recordPair(n, names, u)
		}
		return true
	})
}

// readOnlyReferences finds the references to a reason that only look at
// a reason and never raise it: a comparison (x == reasons.Y), a case of
// a switch. A detector that only reads a reason is not listed for it.
func readOnlyReferences(file *ast.File) map[*ast.SelectorExpr]bool {
	reads := make(map[*ast.SelectorExpr]bool)
	mark := func(expr ast.Expr) {
		if sel, ok := expr.(*ast.SelectorExpr); ok {
			reads[sel] = true
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				mark(n.X)
				mark(n.Y)
			}
		case *ast.CaseClause:
			for _, expr := range n.List {
				mark(expr)
			}
		}
		return true
	})
	return reads
}

// recordLiteral pairs Reason and Severity fields of one struct literal.
func recordLiteral(
	lit *ast.CompositeLit, names map[string]string, u detectorUsage,
) {
	var reason, severity string
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Reason":
			reason = selectorReason(kv.Value, names)
		case "Severity":
			severity = severityName(kv.Value)
		}
	}
	if reason != "" && severity != "" {
		add(u.severities, reason, severity)
	}
}

// recordPair handles tables keyed by reason with a severity value.
func recordPair(
	kv *ast.KeyValueExpr, names map[string]string, u detectorUsage,
) {
	reason := selectorReason(kv.Key, names)
	severity := severityName(kv.Value)
	if reason != "" && severity != "" {
		add(u.severities, reason, severity)
	}
}

func selectorReason(expr ast.Expr, names map[string]string) string {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		v, _ := reasonValue(sel, names)
		return v
	}
	return ""
}

func reasonValue(
	sel *ast.SelectorExpr, names map[string]string,
) (string, bool) {
	if id, ok := sel.X.(*ast.Ident); ok && id.Name == "reasons" {
		v, found := names[sel.Sel.Name]
		return v, found
	}
	return "", false
}

func severityName(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if id, ok := sel.X.(*ast.Ident); ok && id.Name == "detection" {
		switch sel.Sel.Name {
		case "Info", "Warning", "Critical":
			return sel.Sel.Name
		}
	}
	return ""
}

// reasonNames maps reason constant names to their values.
func reasonNames(root string) (map[string]string, error) {
	path := filepath.Join(root, "internal", "detection", "reasons",
		"reasons.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
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
			if lit, ok := spec.Values[i].(*ast.BasicLit); ok {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					out[name.Name] = v
				}
			}
		}
		return true
	})
	return out, nil
}

func add(m map[string]map[string]bool, key, value string) {
	if m[key] == nil {
		m[key] = make(map[string]bool)
	}
	m[key][value] = true
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
