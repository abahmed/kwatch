package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

// retiredPackages were removed by ADR 0010, or renamed by its naming note,
// and must not return.
var retiredPackages = []string{
	"internal/controller", "internal/insight",
	"internal/persistence", "internal/handler", "internal/monitor",
	"internal/pvc", "internal/probe", "internal/networkgraph",
	"internal/storagegraph", "internal/statuswatch",
	"internal/graphcontext", "internal/resource",
	"internal/kubeletmetrics", "internal/controlplane",
	"internal/delivery/util", "internal/problem", "internal/reason",
	"internal/signal", "internal/knowledge", "internal/core",
	"internal/story", "internal/notice",
	// Folded or renamed by the package-structure cleanup.
	"internal/model", "internal/message", "internal/constant",
	"internal/k8s", "internal/client", "internal/crdwatch",
	"internal/startup", "internal/filter",
}

func TestRetiredPackagesAreAbsent(t *testing.T) {
	root := repositoryRoot(t)
	for _, retired := range retiredPackages {
		files := goFilesRecursive(t, filepath.Join(root, retired))
		if len(files) != 0 {
			t.Errorf("retired package %s contains Go files: %v",
				retired, files)
		}
		for _, filename := range goFilesRecursive(t, root) {
			for _, importSpec := range parseFile(t, filename).Imports {
				path := strings.Trim(importSpec.Path.Value, "\"")
				if importsPackage(path, retired) {
					t.Errorf("%s imports retired package %s",
						filename, path)
				}
			}
		}
	}
}

func TestOnlyApplicationImportsPipeline(t *testing.T) {
	root := repositoryRoot(t)
	allowed := []string{
		filepath.Join(root, "internal", "app") + string(filepath.Separator),
		filepath.Join(root, "internal", "pipeline") +
			string(filepath.Separator),
		// replay is test tooling that drives an engine through a log.
		filepath.Join(root, "internal", "replay") +
			string(filepath.Separator),
	}
	for _, filename := range goFilesRecursive(t, root) {
		if isUnder(filename, allowed) {
			continue
		}
		for _, importSpec := range parseFile(t, filename).Imports {
			path := strings.Trim(importSpec.Path.Value, "\"")
			if importsPackage(path, "internal/pipeline") {
				t.Errorf("%s imports the pipeline outside the app", filename)
			}
		}
	}
}

func isUnder(filename string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(filename, prefix) {
			return true
		}
	}
	return false
}

// importsPackage reports whether an import path is the internal package or
// one of its subpackages. Prefix matching keeps internal/alert/signal apart
// from the retired internal/signal.
func importsPackage(path, internal string) bool {
	full := modulePath + internal
	return path == full || strings.HasPrefix(path, full+"/")
}

// replay is test tooling: production code must never import it.
func TestOnlyTestsImportReplay(t *testing.T) {
	root := repositoryRoot(t)
	allowed := []string{
		filepath.Join(root, "internal", "replay") +
			string(filepath.Separator),
	}
	for _, filename := range goFilesRecursive(t, root) {
		if isUnder(filename, allowed) {
			continue
		}
		for _, importSpec := range parseFile(t, filename).Imports {
			path := strings.Trim(importSpec.Path.Value, "\"")
			if importsPackage(path, "internal/replay") {
				t.Errorf("%s imports replay outside tests", filename)
			}
		}
	}
}
