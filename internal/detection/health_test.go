package detection

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestHealthForSeverity(t *testing.T) {
	cases := []struct {
		severity Severity
		want     Health
	}{
		{Critical, Failing},
		{Warning, Degraded},
		{Info, Degraded},
		{0, Unknown},
		{Severity(99), Unknown},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, HealthFor(tc.severity), tc.severity)
	}
}

func TestHealthString(t *testing.T) {
	cases := map[Health]string{
		Healthy: "healthy", Degraded: "degraded", Failing: "failing",
		Unknown: "unknown", 0: "unclassified",
	}
	for health, want := range cases {
		assert.Equal(t, want, health.String())
	}
}

func TestModeForReason(t *testing.T) {
	cases := map[string]string{
		reasons.CrashLoopBackOff:      "CrashLoop",
		reasons.ErrImagePull:          "ImagePull",
		reasons.ImagePullBackOff:      "ImagePull",
		reasons.OOMKilled:             "OOMKilled",
		reasons.OOMKILLED:             "OOMKilled",
		reasons.MemoryPressure:        "MemoryPressure",
		reasons.NodeMemoryPressure:    "MemoryPressure",
		reasons.CustomResourceFailure: "NotReconciling",
		reasons.PodStuckTerminating:   "StuckDeleting",
		reasons.NamespaceStuck:        "StuckDeleting",
		"RunContainerError":           "RunContainerError",
	}
	for reason, want := range cases {
		assert.Equal(t, want, string(ModeFor(reason)), reason)
	}
}

func TestClassifyKeepsDetectorValues(t *testing.T) {
	got := Classify(Finding{Reason: reasons.ErrImagePull,
		Severity: Critical})
	assert.Equal(t, Failing, got.Health)
	assert.Equal(t, "ImagePull", string(got.Mode))

	set := Finding{Reason: reasons.ErrImagePull, Severity: Critical,
		Health: Unknown, Mode: "ImagePull.Auth"}
	assert.Equal(t, set, Classify(set))
	assert.Equal(t, got, Classify(got), "classify is idempotent")
}

func TestEveryReasonHasMode(t *testing.T) {
	count := 0
	for _, decl := range reasonDecls(t) {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				lit, ok := value.(*ast.BasicLit)
				require.True(t, ok, "reason constants are literals")
				reason, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				count++
				mode, ok := modeByReason[reason]
				assert.True(t, ok, "reason %q has no mode", reason)
				assert.NotEmpty(t, mode, reason)
			}
		}
	}
	assert.Equal(t, count, len(modeByReason),
		"every mode entry names a reason constant")
}

// reasonDecls returns the declarations of every non-test file in the
// reasons package, so reasons can be split across files by area.
func reasonDecls(t *testing.T) []ast.Decl {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "reasons", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)
	var decls []ast.Decl
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			decls = append(decls, file.Decls...)
		}
	}
	return decls
}

func TestRegistryEvaluateClassifiesFindings(t *testing.T) {
	d := &mockDetectorWithFinding{
		name:  "any",
		kinds: []inventory.Kind{AnyKind},
		findings: []Finding{
			{Reason: reasons.CrashLoopBackOff, Severity: Critical},
			{Reason: "Custom", Severity: Info},
		},
	}
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	eval := NewRegistry(nil, d).Evaluate(
		inventory.NewModel(inventory.Options{}), time.Now(), id)
	require.Len(t, eval.Findings, 2)
	assert.Equal(t, Failing, eval.Findings[0].Health)
	assert.Equal(t, "CrashLoop", string(eval.Findings[0].Mode))
	assert.Equal(t, Degraded, eval.Findings[1].Health)
	assert.Equal(t, "Custom", string(eval.Findings[1].Mode))
}
