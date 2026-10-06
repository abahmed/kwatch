package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

// pipelineParts are the subpackages of the pipeline. The pipeline wires
// them, so each is a leaf: it never imports the pipeline itself or a
// sibling.
var pipelineParts = []string{"announce", "coverage", "investigate"}

func TestPipelineSubpackagesAreLeaves(t *testing.T) {
	root := repositoryRoot(t)
	for _, part := range pipelineParts {
		dir := filepath.Join(root, "internal", "pipeline", part)
		for _, filename := range goFilesRecursive(t, dir) {
			for _, spec := range parseFile(t, filename).Imports {
				path := strings.Trim(spec.Path.Value, "\"")
				if forbiddenForPart(path, part) {
					t.Errorf("%s imports %s: pipeline subpackages "+
						"must not import the pipeline or each other",
						filename, path)
				}
			}
		}
	}
}

// forbiddenForPart reports an import of the pipeline package itself or of
// a sibling subpackage.
func forbiddenForPart(path, part string) bool {
	if path == modulePath+"internal/pipeline" {
		return true
	}
	for _, other := range pipelineParts {
		if other != part &&
			importsPackage(path, "internal/pipeline/"+other) {
			return true
		}
	}
	return false
}
