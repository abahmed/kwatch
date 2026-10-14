package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// pkgInfo holds what one package directory declares: its string
// constants and variables, by name, as expressions to resolve lazily.
type pkgInfo struct {
	importPath string
	decls      map[string]valueDecl
}

type valueDecl struct {
	expr ast.Expr
	file *ast.File
}

// corpus is every parsed non-test Go file under the scanned roots.
type corpus struct {
	root       string
	fset       *token.FileSet
	pkgs       map[string]*pkgInfo // by import path
	files      map[string]*ast.File
	fileImport map[string]string // file relpath -> import path
	calls      []callSite
	unresolved []string
	untyped    []callSite
	topLevel   map[*ast.ValueSpec]bool
}

// importPathOf maps a directory of the checkout to its import path.
func importPathOf(dir string) string {
	const staging = "staging/src/"
	if rest, ok := strings.CutPrefix(dir, staging); ok {
		return rest
	}
	return "k8s.io/kubernetes/" + dir
}

func load(root string, dirs []string) (*corpus, error) {
	c := &corpus{root: root, fset: token.NewFileSet(),
		pkgs: map[string]*pkgInfo{}, files: map[string]*ast.File{},
		fileImport: map[string]string{},
		topLevel:   map[*ast.ValueSpec]bool{}}
	for _, dir := range dirs {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry,
			err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") ||
				strings.HasSuffix(p, "_test.go") {
				return err
			}
			return c.parse(p)
		})
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *corpus) parse(p string) error {
	f, err := parser.ParseFile(c.fset, p, nil, 0)
	if err != nil {
		return err
	}
	rel, _ := filepath.Rel(c.root, p)
	rel = filepath.ToSlash(rel)
	ip := importPathOf(path.Dir(rel))
	c.files[rel] = f
	c.fileImport[rel] = ip
	info := c.pkgs[ip]
	if info == nil {
		info = &pkgInfo{importPath: ip, decls: map[string]valueDecl{}}
		c.pkgs[ip] = info
	}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			c.topLevel[vs] = true
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					info.decls[n.Name] = valueDecl{vs.Values[i], f}
				}
			}
		}
	}
	return nil
}

// resolve returns the string an expression stands for: a literal, an
// errors.New wrapper, a conversion, or a
// reference to another constant or variable of the corpus.
func (c *corpus) resolve(
	e ast.Expr, f *ast.File, ip string, depth int,
) (string, bool) {
	if depth > 8 {
		return "", false
	}
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.ParenExpr:
		return c.resolve(v.X, f, ip, depth+1)
	case *ast.CallExpr:
		return c.resolveCall(v, f, ip, depth)
	case *ast.Ident:
		if c.isLocal(v) {
			return "", false
		}
		d, ok := c.pkgs[ip].decls[v.Name]
		if !ok {
			return "", false
		}
		return c.resolve(d.expr, d.file, ip, depth+1)
	case *ast.SelectorExpr:
		x, ok := v.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		target := importFor(f, x.Name)
		info := c.pkgs[target]
		if info == nil {
			return "", false
		}
		d, ok := info.decls[v.Sel.Name]
		if !ok {
			return "", false
		}
		return c.resolve(d.expr, d.file, target, depth+1)
	}
	return "", false
}

// isLocal reports whether an identifier names a parameter or a local
// variable or constant, which shadows a package-level name.
func (c *corpus) isLocal(id *ast.Ident) bool {
	if id.Obj == nil {
		return false
	}
	vs, ok := id.Obj.Decl.(*ast.ValueSpec)
	return !ok || !c.topLevel[vs]
}

// resolveCall unwraps errors.New("x") and conversions such as
// string(X) or v1.PodReason("x").
func (c *corpus) resolveCall(
	call *ast.CallExpr, f *ast.File, ip string, depth int,
) (string, bool) {
	if len(call.Args) != 1 {
		return "", false
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "errors" &&
			sel.Sel.Name == "New" {
			return c.resolve(call.Args[0], f, ip, depth+1)
		}
	}
	switch call.Fun.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return c.resolve(call.Args[0], f, ip, depth+1)
	}
	return "", false
}

// importFor finds the import path a file binds to a package name.
func importFor(f *ast.File, name string) string {
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if imp.Name != nil && imp.Name.Name == name {
			return p
		}
		if imp.Name == nil && path.Base(p) == name {
			return p
		}
	}
	return ""
}

func (c *corpus) pos(n ast.Node) string {
	p := c.fset.Position(n.Pos())
	rel, _ := filepath.Rel(c.root, p.Filename)
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), p.Line)
}
