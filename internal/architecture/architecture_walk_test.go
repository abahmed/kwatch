package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isNestedRuleDir reports whether filename belongs to a subpackage that has
// its own entry in forbiddenImports; that entry governs it instead. This
// keeps the leaf notification rules from applying to notification/compose.
func isNestedRuleDir(root, packageDir, filename string) bool {
	for other := range forbiddenImports {
		if other == packageDir ||
			!strings.HasPrefix(other, packageDir+"/") {
			continue
		}
		prefix := filepath.Join(root, other) + string(filepath.Separator)
		if strings.HasPrefix(filename, prefix) {
			return true
		}
	}
	return false
}

func TestGoFilesRecursiveMissingDirectoryIsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "retired")
	if files := goFilesRecursive(t, dir); len(files) != 0 {
		t.Fatalf("missing directory returned Go files: %v", files)
	}
}

func TestGoFilesRecursiveEmptyDirectoryIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if files := goFilesRecursive(t, dir); len(files) != 0 {
		t.Fatalf("empty directory returned Go files: %v", files)
	}
}

func TestGoFilesRecursiveFindsProductionFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "retired")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	filename := filepath.Join(dir, "nested", "runtime.go")
	contents := []byte("package runtime\n")
	if err := os.WriteFile(filename, contents, 0o644); err != nil {
		t.Fatalf("write test Go file: %v", err)
	}
	files := goFilesRecursive(t, dir)
	if len(files) != 1 || files[0] != filename {
		t.Fatalf("found Go files = %v, want [%s]", files, filename)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func goFiles(t *testing.T, dir string) []string {
	return goFilesRecursive(t, dir)
}

func goFilesRecursive(t *testing.T, dir string) []string {
	t.Helper()
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return []string{}
		}
		t.Fatalf("stat Go files in %s: %v", dir, err)
	}
	files := make([]string, 0)
	err := filepath.WalkDir(dir, func(
		path string, entry fs.DirEntry, err error,
	) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") ||
			!strings.HasSuffix(path, ".go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("find Go files in %s: %v", dir, err)
	}
	return files
}

func parseFile(t *testing.T, filename string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	return file
}
