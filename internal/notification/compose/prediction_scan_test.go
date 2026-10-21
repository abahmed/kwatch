package compose

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

// forecastWords make a sentence say what may happen instead of what
// is happening. kwatch reports the present: a note states the facts.
var forecastWords = regexp.MustCompile(
	`(?i)\b(will|won't|may|might|could|likely|soon|would|probably|` +
		`possibly|eventually|about to|going to)\b`)

// factualWording are the few strings that use such a word without
// predicting anything, each with the reason. A literal is cleared when it
// contains a key.
var factualWording = map[string]string{
	// Past tense or kwatch's own failed attempt.
	"could not":   "says what kwatch tried and could not do",
	"soon after ": "orders two events that already happened",
	// The confidence scale (confidence.go) hedges a diagnosis made now,
	// as docs/contributing-message-fact.md asks for.
	", likely because ":  "confidence wording of a diagnosis",
	", likely caused by": "confidence wording of a diagnosis",
	"might be related":   "confidence wording of a diagnosis",
	"the likely cause":   "confidence wording of a diagnosis",
	// A hypothesis about a state kwatch cannot observe now.
	"may be down or unreachable":        "a node that stopped reporting",
	"nodes may be down or ":             "nodes that stopped reporting",
	" (it may run outside the cluster)": "a provisioner kwatch cannot see",
	// What kwatch itself does next, not what the cluster will do.
	"kwatch will write again": "kwatch's own behaviour",
	// The fix named for a failure that is happening.
	"would let it start": "names what ends the present failure",
	// States a LimitRange rule that is in force now.
	"limit may be at most": "the rule the namespace sets",
}

// factualLiterals are whole literals that are words of the code, not of a
// note.
var factualLiterals = map[string]string{
	"could ": "a verb the predicate parser strips",
	"likely": "the name of a confidence level",
}

// TestComposeTextStatesFactsNotPredictions scans every string literal
// the writers can put in a note for a word that predicts, the same scan
// the detectors have. A new sentence that writes "may fail" fails here:
// say what is true now.
func TestComposeTextStatesFactsNotPredictions(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, text := range composeLiterals(t, file) {
			if composeForecasts(text) {
				t.Errorf("%s: %q predicts instead of stating a fact",
					file, text)
			}
		}
	}
}

func composeForecasts(text string) bool {
	if _, ok := factualLiterals[text]; ok {
		return false
	}
	for allowed := range factualWording {
		text = strings.ReplaceAll(text, allowed, " ")
	}
	return forecastWords.MatchString(text)
}

func composeLiterals(t *testing.T, file string) []string {
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
