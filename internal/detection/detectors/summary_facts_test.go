package detectors

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// predictionWords make a summary say what may happen instead of what is
// happening. kwatch reports the present: a finding states the facts.
var predictionWords = regexp.MustCompile(
	`(?i)\b(will|won't|may|might|could|likely|soon|would|probably|` +
		`possibly|eventually|about to|going to)\b`)

// factualWording are the few strings that use such a word without
// predicting anything, each with the reason.
var factualWording = map[string]string{
	// Past tense about kwatch's own failed attempt.
	"could not": "says what kwatch tried and could not do",
	// What-if evidence: the scheduler check names the one change that
	// would let the pod fit, which is its purpose.
	" would fit it on ": "names the node a lifted constraint would fit",
	"fit would":         "label of that evidence",
}

// TestDetectorTextStatesFactsNotPredictions scans every string literal
// the detectors and the reason tables can put in a finding for a word
// that predicts. A new detector that writes "may fail" fails here: say
// what is true now, or quote what Kubernetes said.
func TestDetectorTextStatesFactsNotPredictions(t *testing.T) {
	for _, dir := range []string{".", ".."} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			for _, text := range stringLiterals(t, file) {
				if predicts(text) {
					t.Errorf("%s: %q predicts instead of stating a fact",
						file, text)
				}
			}
		}
	}
}

func predicts(text string) bool {
	for allowed := range factualWording {
		text = strings.ReplaceAll(text, allowed, " ")
	}
	return predictionWords.MatchString(text)
}

func stringLiterals(t *testing.T, file string) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	require.NoError(t, err)
	parsed, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
	require.NoError(t, err)
	var out []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if text, err := strconv.Unquote(lit.Value); err == nil {
			out = append(out, text)
		}
		return true
	})
	return out
}
