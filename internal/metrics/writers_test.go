package metrics

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// writtenVia maps Registry fields that production code updates through a
// helper method instead of touching the field directly.
var writtenVia = map[string]string{
	"IncidentActions":   "IncIncident",
	"TelemetryFailures": "IncTelemetryFailure",
	"StorageResets":     "IncStorageReset",
	"Investigations":    "IncInvestigation",
	"DecisionLag":       "ObserveDecisionLag",
}

// productionSelectors returns every selector name used by non-test Go files
// outside this package. A metric with no writer never appears in this set.
func productionSelectors(t *testing.T) map[string]bool {
	t.Helper()
	used := map[string]bool{}
	fset := token.NewFileSet()
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root,
			func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				slash := filepath.ToSlash(path)
				if d.IsDir() && strings.HasSuffix(slash, "internal/metrics") {
					return filepath.SkipDir
				}
				if d.IsDir() || !strings.HasSuffix(path, ".go") ||
					strings.HasSuffix(path, "_test.go") {
					return nil
				}
				file, perr := parser.ParseFile(fset, path, nil, 0)
				if perr != nil {
					return perr
				}
				ast.Inspect(file, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok {
						used[sel.Sel.Name] = true
					}
					return true
				})
				return nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}
	return used
}

func TestRegistryFieldsHaveWriters(t *testing.T) {
	used := productionSelectors(t)
	typ := reflect.TypeOf(Registry{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Name
		if helper, ok := writtenVia[name]; ok {
			name = helper
		}
		if !used[name] {
			t.Errorf("metric field %s has no production writer; "+
				"delete it or wire it (%s)", field.Name, name)
		}
	}
}

func TestEveryDescribedMetricIsCollected(t *testing.T) {
	r := &Registry{}
	ch := make(chan *prometheus.Desc, 256)
	r.Describe(ch)
	close(ch)
	want := len(ch)
	got := strings.Count(scrape(t, r), "# TYPE kwatch_")
	if got != want {
		t.Fatalf("collected %d kwatch metrics, described %d", got, want)
	}
}
