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

var forbiddenImports = map[string][]string{
	"internal/monitor": {
		"internal/app", "internal/controller", "internal/handler",
		"internal/delivery", "internal/alert", "internal/persistence",
		"internal/startup", "internal/upgrader", "internal/k8s",
	},
	"internal/monitor/cluster": {
		"internal/monitor/security", "internal/app", "internal/controller",
		"internal/delivery", "internal/alert", "internal/persistence",
		"internal/k8s",
	},
	"internal/monitor/security": {
		"internal/monitor/network", "internal/app", "internal/controller",
		"internal/delivery", "internal/alert", "internal/persistence",
		"internal/k8s",
	},
	"internal/monitor/node": {
		"internal/incident", "internal/delivery", "internal/persistence",
		"internal/k8s",
	},
	"internal/monitor/network": {
		"internal/incident", "internal/delivery", "internal/persistence",
		"internal/k8s",
	},
	"internal/monitor/pod": {
		"internal/app", "internal/controller", "internal/delivery",
		"internal/alert", "internal/persistence", "internal/k8s",
	},
	"internal/monitor/workload": {
		"internal/app", "internal/controller", "internal/delivery",
		"internal/alert", "internal/persistence", "internal/k8s",
	},
	"internal/alert": {
		"internal/app", "internal/controller", "internal/handler",
		"internal/incident", "internal/persistence", "internal/k8s",
	},
	"internal/delivery": {"internal/alert/"},
	"internal/incident": {
		"internal/audit", "internal/delivery", "internal/persistence",
	},
	"internal/insight": {
		"internal/audit", "internal/delivery", "internal/persistence",
		"internal/k8s",
	},
	"internal/persistence": {
		"internal/controller", "internal/delivery", "internal/handler",
		"internal/insight",
	},
	"internal/pvc":            {"internal/persistence"},
	"internal/kubeletmetrics": {"internal/incident"},
	"internal/metricsapi":     {"internal/incident"},
	"internal/probe":          {"internal/incident"},
	"internal/resource":       {"internal/incident"},
	"internal/rbac":           {"internal/incident"},
	"internal/controlplane":   {"internal/incident"},
	"internal/statuswatch":    {"internal/incident"},
	"internal/filter": {
		"internal/alert", "internal/app", "internal/controller",
		"internal/delivery", "internal/handler", "internal/incident",
		"internal/insight", "internal/k8s", "internal/persistence",
	},
}

func TestCompatibilityClientConstructionIsExplicit(t *testing.T) {
	root := repositoryRoot(t)
	directories := []string{
		"internal/networkgraph", "internal/storagegraph",
		"internal/statuswatch", "internal/metricsapi", "internal/controlplane",
		"internal/crdwatch",
	}
	for _, directory := range directories {
		files := goFiles(t, filepath.Join(root, directory))
		for _, filename := range files {
			if filepath.Base(filename) == "compat.go" {
				continue
			}
			contents, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("read %s: %v", filename, err)
			}
			for _, forbidden := range []string{
				"dynamic.NewForConfig", "discovery.NewDiscoveryClientForConfig",
				"rest.RESTClientFor",
			} {
				if strings.Contains(string(contents), forbidden) {
					t.Errorf(
						"%s constructs %s outside compat.go",
						filename, forbidden,
					)
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
	dynamicwatch := filepath.Join(root, "internal", "k8s", "dynamicwatch")
	for _, filename := range goFilesRecursive(t, filepath.Join(root, "internal")) {
		if strings.HasPrefix(filename, dynamicwatch) ||
			filepath.Base(filename) == "compat.go" {
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
		"internal/config/", "internal/app/", "internal/crdwatch/",
		"internal/controller/compat.go", "internal/delivery/compat.go",
		"internal/rbac/compat.go", "internal/client/compat.go",
		"cmd/configcatalog/",
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

func TestCompatibilityConstructorsStayInCompatibilityFiles(t *testing.T) {
	root := repositoryRoot(t)
	checks := []struct {
		name   string
		needle string
	}{
		{
			name:   "incident engine",
			needle: "incident.NewEngine(",
		},
		{
			name:   "clock fallback",
			needle: "clock.From(",
		},
	}
	for _, check := range checks {
		for _, filename := range goFilesRecursive(t, root) {
			if filepath.Base(filename) == "compat.go" ||
				strings.HasSuffix(filename, "_test.go") ||
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
				t.Errorf(
					"%s compatibility constructor used outside compat.go: %s",
					check.name, filename,
				)
			}
		}
	}
}

func TestPackageDependenciesFollowOwnershipRules(t *testing.T) {
	root := repositoryRoot(t)
	for packageDir, forbidden := range forbiddenImports {
		files := goFiles(t, filepath.Join(root, packageDir))
		for _, filename := range files {
			file := parseFile(t, filename)
			for _, importSpec := range file.Imports {
				path := strings.Trim(importSpec.Path.Value, "\"")
				if !strings.HasPrefix(path, modulePath) {
					continue
				}
				for _, forbiddenPath := range forbidden {
					if strings.Contains(path, forbiddenPath) {
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

func TestRetiredMonitorRuntimePackageHasNoProductionCode(t *testing.T) {
	root := repositoryRoot(t)
	dir := filepath.Join(root, "internal", "monitor", "runtime")
	files := goFiles(t, dir)
	if len(files) != 0 {
		t.Fatalf("retired monitor runtime package contains Go files: %v", files)
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
