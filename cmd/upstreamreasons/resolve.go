package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

// fromRef resolves a "dir:Name" ref to the string it declares.
func (c *corpus) fromRef(ref string) (string, error) {
	dir, name, ok := strings.Cut(ref, ":")
	if !ok {
		return "", fmt.Errorf("bad ref %q", ref)
	}
	info := c.pkgs[importPathOf(dir)]
	if info == nil {
		return "", fmt.Errorf("ref %s: directory not loaded", ref)
	}
	d, ok := info.decls[name]
	if !ok {
		return "", fmt.Errorf("ref %s: no such constant", ref)
	}
	v, ok := c.resolve(d.expr, d.file, info.importPath, 0)
	if !ok {
		return "", fmt.Errorf("ref %s: not a string constant", ref)
	}
	return v, nil
}

// hasLiteral reports whether the file contains the string literal.
func (c *corpus) hasLiteral(file, value string) bool {
	f := c.files[file]
	if f == nil {
		return false
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if s, err := strconv.Unquote(bl.Value); err == nil &&
				s == value {
				found = true
			}
		}
		return !found
	})
	return found
}

// values resolves a group; the error lists everything that no longer
// resolves, so one run shows the whole drift.
func (c *corpus) values(g group) ([]string, error) {
	var out, bad []string
	for _, ref := range g.refs {
		v, err := c.fromRef(ref)
		if err != nil {
			bad = append(bad, err.Error())
			continue
		}
		out = append(out, v)
	}
	for _, l := range g.lits {
		if !c.hasLiteral(l.file, l.value) {
			bad = append(bad, fmt.Sprintf("%s: literal %q gone",
				l.file, l.value))
			continue
		}
		out = append(out, l.value)
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(bad, "\n"))
	}
	return sortedUnique(out), nil
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// addDynamic adds the events of the unresolved call sites the manifest
// explains, and fails on a call site it does not.
func (c *corpus) addDynamic(events map[string]bool) error {
	var bad []string
	for _, where := range c.unresolved {
		file, _, _ := strings.Cut(where, ":")
		if _, ok := dynamicEvents[file]; !ok {
			bad = append(bad, "unexplained dynamic reason at "+where)
		}
	}
	for file, list := range dynamicEvents {
		for _, d := range list {
			for _, ev := range extraEvidence[file] {
				if !c.hasLiteral(file, ev) {
					bad = append(bad, fmt.Sprintf(
						"%s: literal %q gone", file, ev))
				}
			}
			vals, err := c.values(group{d.refs, d.lits})
			if err != nil {
				bad = append(bad, err.Error())
			}
			vals = append(vals, d.extra...)
			for _, v := range vals {
				events[v] = events[v] || d.warn
			}
		}
	}
	for _, u := range c.untyped {
		if _, ok := events[u.reason]; !ok {
			bad = append(bad, fmt.Sprintf("%s: reason %q is emitted "+
				"with a variable event type and is not explained",
				u.where, u.reason))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%s", strings.Join(bad, "\n"))
	}
	return nil
}

// conditionReasons resolves the reason argument of every call to the
// named functions; a call whose reason is not a constant is an error.
func (c *corpus) conditionReasons(calls []conditionCall) ([]string, error) {
	var out, bad []string
	for rel, f := range c.files {
		ip := c.fileImport[rel]
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			for _, cc := range calls {
				if importPathOf(cc.dir) != ip ||
					calleeName(call) != cc.fn {
					continue
				}
				v, ok := c.resolve(call.Args[cc.arg], f, ip, 0)
				if !ok {
					// Reasons computed at run time belong to healthy
					// conditions (stabilization, scaling limits). One
					// that sets a failure condition must be a constant.
					if isFailureCondition(call) {
						bad = append(bad, "failure condition reason is "+
							"not a constant at "+c.pos(call))
					}
					continue
				}
				out = append(out, v)
			}
			return true
		})
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(bad, "\n"))
	}
	return sortedUnique(out), nil
}

// isFailureCondition reports whether the call sets AbleToScale or
// ScalingActive to False, the conditions kwatch reads for failures.
func isFailureCondition(call *ast.CallExpr) bool {
	var isFalse, isHealth bool
	for _, a := range call.Args {
		if sel, ok := a.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "ConditionFalse":
				isFalse = true
			case "AbleToScale", "ScalingActive":
				isHealth = true
			}
		}
	}
	return isFalse && isHealth
}
