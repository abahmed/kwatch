package main

import (
	"go/ast"
	"sort"
	"strings"
)

// callSite is one event emission: the reason and its type.
type callSite struct {
	reason  string
	warning bool
	where   string
}

// wrapper is a function that records an event whose reason is one of
// its own parameters: the reasons are then in its callers.
type wrapper struct {
	param   int
	warning bool
}

// eventTypeOf reports whether the expression is the Warning or Normal
// event type constant (v1.EventTypeWarning and the like).
func eventTypeOf(e ast.Expr) (warning, ok bool) {
	name := ""
	switch v := e.(type) {
	case *ast.SelectorExpr:
		name = v.Sel.Name
	case *ast.Ident:
		name = v.Name
	}
	switch name {
	case "EventTypeWarning":
		return true, true
	case "EventTypeNormal":
		return false, true
	}
	return false, false
}

func calleeName(call *ast.CallExpr) string {
	switch v := call.Fun.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.Ident:
		return v.Name
	}
	return ""
}

// collectCalls records every call that passes a literal event type
// followed by the reason: the recorder's Event, Eventf and
// AnnotatedEventf, and the wrappers around them. When the reason is a
// parameter of the enclosing function, that function is a wrapper and
// its callers are read for the reasons. Any other reason that is not a
// constant is listed as unresolved, so a new dynamic call site is a
// visible decision and not a silent gap.
func (c *corpus) collectCalls(skip func(file string) bool) {
	wrappers := map[string]wrapper{}
	for rel, f := range c.files {
		if skip(rel) {
			continue
		}
		ip := c.fileImport[rel]
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			params := paramIndex(fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					c.typedCall(call, f, ip, fn.Name.Name, params,
						wrappers)
				}
				return true
			})
		}
	}
	c.callWrappers(wrappers, skip)
	sort.Strings(c.unresolved)
}

func paramIndex(fn *ast.FuncDecl) map[string]int {
	out := map[string]int{}
	i := 0
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			out[n.Name] = i
			i++
		}
	}
	return out
}

func (c *corpus) typedCall(
	call *ast.CallExpr, f *ast.File, ip, fnName string,
	params map[string]int, wrappers map[string]wrapper,
) {
	for i, a := range call.Args {
		warning, isType := eventTypeOf(a)
		if !isType || i+1 >= len(call.Args) {
			continue
		}
		arg := call.Args[i+1]
		if reason, ok := c.resolve(arg, f, ip, 0); ok {
			c.calls = append(c.calls,
				callSite{reason, warning, c.pos(call)})
			return
		}
		if id, ok := arg.(*ast.Ident); ok {
			if idx, isParam := params[id.Name]; isParam {
				wrappers[fnName] = wrapper{idx, warning}
				return
			}
		}
		c.unresolved = append(c.unresolved, c.pos(call))
		return
	}
	c.untypedCall(call, f, ip)
}

// recorderReasonArg is where the reason sits in the recorder methods
// when the event type is not a constant.
var recorderReasonArg = map[string]int{
	"Event": 2, "Eventf": 2, "AnnotatedEventf": 3,
}

// untypedCall notes a recorder call whose event type is a variable but
// whose reason is a constant. Its type is not known at the call.
func (c *corpus) untypedCall(call *ast.CallExpr, f *ast.File, ip string) {
	i, ok := recorderReasonArg[calleeName(call)]
	if !ok || i >= len(call.Args) {
		return
	}
	if reason, ok := c.resolve(call.Args[i], f, ip, 0); ok {
		c.untyped = append(c.untyped, callSite{reason, false, c.pos(call)})
	}
}

// callWrappers resolves the reason argument at every call of a wrapper.
func (c *corpus) callWrappers(
	wrappers map[string]wrapper, skip func(string) bool,
) {
	for rel, f := range c.files {
		if skip(rel) {
			continue
		}
		ip := c.fileImport[rel]
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			w, ok := wrappers[calleeName(call)]
			if !ok || w.param >= len(call.Args) {
				return true
			}
			if reason, ok := c.resolve(call.Args[w.param], f, ip, 0); ok {
				c.calls = append(c.calls,
					callSite{reason, w.warning, c.pos(call)})
			} else {
				c.unresolved = append(c.unresolved,
					c.pos(call)+" (via "+calleeName(call)+")")
			}
			return true
		})
	}
}

// eventReasons folds the call sites into one entry per reason; a
// reason any call site records as Warning is a Warning.
func eventReasons(calls []callSite) map[string]bool {
	out := map[string]bool{}
	for _, cs := range calls {
		out[cs.reason] = out[cs.reason] || cs.warning
	}
	return out
}

func isGenerated(file string) bool {
	return strings.Contains(file, "/generated") ||
		strings.Contains(file, "/testing/") ||
		strings.Contains(file, "/testutil/")
}
