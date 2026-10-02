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

const modulePath = "github.com/abahmed/kwatch/"

// forbiddenImports lists, per package directory, the internal packages it
// must not import. Dependencies flow downward:
// inventory -> detection -> rootcause -> incident -> notification/compose
// -> pipeline -> app. notification and storage are leaves.
var forbiddenImports = map[string][]string{
	"internal/inventory": {
		"internal/detection", "internal/rootcause", "internal/incident",
		"internal/notification", "internal/pipeline", "internal/storage",
		"internal/scope", "internal/delivery", "internal/alert",
		"internal/app", "internal/health", "internal/rbac",
	},
	"internal/detection": {
		"internal/rootcause", "internal/incident",
		"internal/notification", "internal/pipeline", "internal/storage",
		"internal/scope", "internal/delivery", "internal/alert",
		"internal/app",
	},
	"internal/rootcause": {
		"internal/incident", "internal/notification", "internal/pipeline",
		"internal/storage", "internal/scope", "internal/delivery",
		"internal/alert", "internal/app",
	},
	"internal/incident": {
		"internal/notification", "internal/pipeline", "internal/storage",
		"internal/scope", "internal/delivery", "internal/alert",
		"internal/app",
	},
	"internal/notification/compose": {
		"internal/pipeline", "internal/storage", "internal/scope",
		"internal/delivery", "internal/alert", "internal/app",
	},
	"internal/notification": {
		"internal/notification/compose", "internal/incident",
		"internal/rootcause", "internal/detection", "internal/inventory",
		"internal/pipeline", "internal/storage", "internal/delivery",
		"internal/alert", "internal/app",
	},
	"internal/storage": {
		"internal/inventory", "internal/detection", "internal/rootcause",
		"internal/incident", "internal/notification", "internal/pipeline",
		"internal/delivery", "internal/alert", "internal/app",
	},
	"internal/scope": {
		"internal/rootcause", "internal/incident", "internal/notification",
		"internal/pipeline", "internal/delivery", "internal/alert",
		"internal/app",
	},
	"internal/pipeline": {
		"internal/delivery", "internal/alert", "internal/app",
		"internal/health", "internal/config",
	},
	// replay is test tooling above the pipeline: it drives an engine and
	// never reaches delivery, persistence, configuration or clusters.
	"internal/replay": {
		"internal/app", "internal/delivery", "internal/alert",
		"internal/health", "internal/config", "internal/storage",
		"internal/scope", "internal/audit", "internal/kubeclient",
		"internal/rbac", "internal/inventory/kube",
	},
	"internal/rbac": {
		"internal/detection", "internal/rootcause", "internal/incident",
		"internal/notification", "internal/pipeline", "internal/delivery",
		"internal/alert", "internal/app",
	},
	"internal/alert": {
		"internal/app", "internal/pipeline", "internal/incident",
		"internal/rootcause", "internal/detection",
		"internal/notification/compose", "internal/inventory",
		"internal/storage", "internal/scope", "internal/kubeclient",
		"internal/config", "internal/health",
		"internal/audit",
	},
	"internal/delivery": {
		"internal/app", "internal/pipeline", "internal/incident",
		"internal/rootcause", "internal/detection",
		"internal/notification/compose", "internal/inventory",
		"internal/storage", "internal/scope", "internal/kubeclient",
		"internal/alert",
	},
}

func TestClientConstructionHasOneOwner(t *testing.T) {
	root := repositoryRoot(t)
	directories := []string{
		"internal/inventory/kube", "internal/config/crd",
	}
	for _, directory := range directories {
		files := goFiles(t, filepath.Join(root, directory))
		for _, filename := range files {
			contents, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("read %s: %v", filename, err)
			}
			for _, forbidden := range []string{
				"dynamic.NewForConfig", "discovery.NewDiscoveryClientForConfig",
				"rest.RESTClientFor",
			} {
				if strings.Contains(string(contents), forbidden) {
					t.Errorf("%s constructs %s outside client composition",
						filename, forbidden)
				}
			}
		}
	}
}

func TestProvidersDoNotRetainApplicationConfiguration(t *testing.T) {
	root := repositoryRoot(t)
	dir := filepath.Join(root, "internal", "alert")
	for _, filename := range goFilesRecursive(t, dir) {
		file := parseFile(t, filename)
		for _, importSpec := range file.Imports {
			path := strings.Trim(importSpec.Path.Value, "\"")
			if path == modulePath+"internal/config" {
				t.Errorf("provider file imports application config: %s", filename)
			}
		}
	}
}

func TestProductionCodeDoesNotUseGlobalClients(t *testing.T) {
	root := repositoryRoot(t)
	for _, filename := range goFilesRecursive(t, filepath.Join(root, "internal")) {
		contents, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		text := string(contents)
		if strings.Contains(text, "http.DefaultClient") {
			t.Errorf("%s uses the process-wide HTTP client", filename)
		}
		if strings.Contains(text, "net.DefaultResolver") {
			t.Errorf("%s uses the process-wide DNS resolver", filename)
		}
	}
}

func TestDynamicInformerConstructionHasOneOwner(t *testing.T) {
	root := repositoryRoot(t)
	dynamicwatch := filepath.Join(
		root, "internal", "inventory", "kube", "dynamicwatch",
	)
	for _, filename := range goFilesRecursive(t, filepath.Join(root, "internal")) {
		if strings.HasPrefix(filename, dynamicwatch) {
			continue
		}
		file := parseFile(t, filename)
		for _, importSpec := range file.Imports {
			path := strings.Trim(importSpec.Path.Value, "\"")
			if path == "k8s.io/client-go/dynamic/dynamicinformer" {
				t.Errorf("%s constructs dynamic informers outside dynamicwatch",
					filename)
			}
		}
	}
}

func TestRawConfigurationStaysAtApprovedBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	approved := []string{
		"internal/config/", "internal/app/",
		"cmd/configcatalog/", "cmd/kwatch/",
	}
	for _, filename := range goFilesRecursive(t, root) {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		if !strings.Contains(string(contents), "config.Config") {
			continue
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			t.Fatalf("make %s relative: %v", filename, err)
		}
		allowed := false
		for _, prefix := range approved {
			if strings.HasPrefix(relative, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			t.Errorf("%s retains raw config.Config outside a boundary",
				relative)
		}
	}
}

func TestRetiredCompatibilityConstructorsAreAbsent(t *testing.T) {
	root := repositoryRoot(t)
	checks := []struct {
		name   string
		needle string
	}{
		{
			name:   "clock fallback",
			needle: "clock.From(",
		},
		{
			name:   "delivery initializer",
			needle: "InitWithFactory(",
		},
	}
	for _, check := range checks {
		for _, filename := range goFilesRecursive(t, root) {
			if strings.HasSuffix(filename, "_test.go") ||
				strings.HasPrefix(
					filename,
					filepath.Join(root, "internal", "clock"),
				) {
				continue
			}
			contents, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("read %s: %v", filename, err)
			}
			if strings.Contains(string(contents), check.needle) {
				t.Errorf("%s remains in production code: %s", check.name, filename)
			}
		}
	}
}

func TestTransitionalCompatibilityFilesAreAbsent(t *testing.T) {
	root := repositoryRoot(t)
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(
		path string, entry fs.DirEntry, err error,
	) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Base(path) == "compat.go" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("find compatibility files: %v", err)
	}
	if len(files) > 0 {
		t.Fatalf("transitional compatibility files remain: %v", files)
	}
}

func TestPackageDependenciesFollowOwnershipRules(t *testing.T) {
	root := repositoryRoot(t)
	for packageDir, forbidden := range forbiddenImports {
		files := goFiles(t, filepath.Join(root, packageDir))
		for _, filename := range files {
			if isNestedRuleDir(root, packageDir, filename) {
				continue
			}
			file := parseFile(t, filename)
			for _, importSpec := range file.Imports {
				path := strings.Trim(importSpec.Path.Value, "\"")
				if !strings.HasPrefix(path, modulePath) {
					continue
				}
				for _, forbiddenPath := range forbidden {
					if importsPackage(path, forbiddenPath) {
						t.Errorf(
							"%s imports forbidden package %s",
							filename, path,
						)
					}
				}
			}
		}
	}
}

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
