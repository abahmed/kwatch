package architecture

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryPackageHasDocFile keeps each package explainable: it needs a
// doc.go whose package comment says what the package does.
func TestEveryPackageHasDocFile(t *testing.T) {
	root := repositoryRoot(t)
	packages := map[string]bool{}
	for _, top := range []string{"internal", "cmd"} {
		for _, file := range goFilesRecursive(t, filepath.Join(root, top)) {
			if strings.Contains(file, "testdata") {
				continue
			}
			packages[filepath.Dir(file)] = true
		}
	}
	for dir := range packages {
		checkPackageDoc(t, root, dir)
	}
}

func checkPackageDoc(t *testing.T, root, dir string) {
	t.Helper()
	rel, _ := filepath.Rel(root, dir)
	file, err := parser.ParseFile(
		token.NewFileSet(), filepath.Join(dir, "doc.go"), nil,
		parser.ParseComments|parser.PackageClauseOnly,
	)
	if err != nil {
		t.Errorf("%s has no readable doc.go: %v", rel, err)
		return
	}
	if file.Doc == nil || strings.TrimSpace(file.Doc.Text()) == "" {
		t.Errorf("%s/doc.go has no package comment", rel)
	}
}
