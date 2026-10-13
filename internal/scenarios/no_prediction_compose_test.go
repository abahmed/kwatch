package scenarios

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// composeDir holds the code that writes kwatch's messages.
var composeDir = filepath.Join("..", "notification", "compose")

// parserTokens are literals that match words in a summary kwatch is
// reading, not words kwatch writes.
var parserTokens = map[string]bool{"could ": true}

// TestNoPredictionInComposeStrings scans every string literal of the
// compose package. A sentence no scenario renders (a consequence for
// one reason, say) is still kwatch's wording, so it is checked too.
func TestNoPredictionInComposeStrings(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(composeDir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no compose files: %v", err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if parserTokens[text] {
				return true
			}
			for _, hit := range predictionHits(text) {
				t.Errorf("%s: predictive wording: %s",
					fset.Position(lit.Pos()), hit)
			}
			return true
		})
	}
}

// TestComposeScanFindsPredictiveLiterals proves the scan reads literals.
func TestComposeScanFindsPredictiveLiterals(t *testing.T) {
	t.Parallel()
	src := "package p\nvar s = \"Clients will reject it\"\n"
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok {
			text, _ := strconv.Unquote(lit.Value)
			found += len(predictionHits(text))
		}
		return true
	})
	if found != 1 {
		t.Errorf("found %d hits, want 1", found)
	}
}
