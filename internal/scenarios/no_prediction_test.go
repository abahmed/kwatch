package scenarios

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// predictive is the wording kwatch never uses about the cluster. It
// reports what is happening or has happened, never what might, may or
// will happen. Every pattern is case-insensitive and word-bounded.
var predictive = regexp.MustCompile(`(?i)\b(` + strings.Join([]string{
	`will`, `won't`, `would fail`, `could`, `may fail`, `might fail`,
	`at risk`, `risk of`, `likely to`, `expected to`, `soon`,
	`about to`, `running out`, `runs out`, `trending`, `projected`,
	`forecast`, `before it (fails|runs out|crashes)`,
	`fragile`, `weakness`,
	`close to (being|its|the) ((\S+ ){0,3}(limit|quota)|oom|\w+-killed)`,
	`(close to|nearly|almost) (full|its limit)`,
	`(refuses|rejects|stops accepting) writes`,
	`in \d+ (seconds?|minutes?|hours?|days?)`,
}, "|") + `)\b`)

// quoted matches text that is not kwatch's own wording: double-quoted
// application text, code spans and curly-quoted text.
var quoted = regexp.MustCompile("\"[^\"\n]*\"|`[^`\n]*`|“[^”\n]*”")

// presentFacts are phrases that look predictive but describe what
// already happened: the out-of-memory kill, "soon after" a change and
// kwatch's own "could not reach" (a probe that failed), and what kwatch
// itself does next ("kwatch will write again"), not the cluster.
var presentFacts = regexp.MustCompile(`(?i)running out of memory|` +
	`soon after|could not|couldn't|kwatch will write again`)

// predictionHits returns every predictive phrase in kwatch's own
// words. Fenced code blocks (commands) and quoted spans are skipped.
func predictionHits(text string) []string {
	var hits []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		bare := strings.TrimPrefix(strings.TrimPrefix(line, "|"), " ")
		if strings.HasPrefix(bare, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		own := quoted.ReplaceAllString(line, " ")
		own = presentFacts.ReplaceAllString(own, " ")
		if predictive.MatchString(own) {
			hits = append(hits, strings.TrimSpace(line))
		}
	}
	return hits
}

// TestPredictionScannerSkipsOthersWords proves the exclusions work.
func TestPredictionScannerSkipsOthersWords(t *testing.T) {
	t.Parallel()
	skipped := "It fails with \"disk will be full soon\".\n" +
		"| It logs `could not connect`.\n" +
		"| ```\n| kubectl wait --timeout=5s will\n| ```\n"
	if hits := predictionHits(skipped); len(hits) != 0 {
		t.Errorf("quoted or command text flagged: %v", hits)
	}
	hits := predictionHits("The disk will be full in 3 hours.")
	if len(hits) != 1 {
		t.Errorf("prediction not flagged: %v", hits)
	}
	for _, old := range []string{
		"the container is close to being OOM-killed",
		"etcd database is 1.9 GiB, close to its 2 GiB limit",
		"At the limit etcd refuses writes",
	} {
		if len(predictionHits(old)) != 1 {
			t.Errorf("near-limit wording not flagged: %q", old)
		}
	}
}

// TestNoPredictionInGoldens scans every golden kwatch's words are
// kept in, so a prediction cannot slip in with a regenerated file.
func TestNoPredictionInGoldens(t *testing.T) {
	t.Parallel()
	dirs := []string{
		filepath.Join("testdata", "messages"),
		filepath.Join("..", "notification", "compose", "testdata"),
	}
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.golden"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no goldens in %s: %v", dir, err)
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range predictionHits(string(data)) {
				t.Errorf("%s: predictive wording: %s", f, h)
			}
		}
	}
}

// TestNoPredictionInRenderedScenarios renders every scenario now, so
// new wording is caught before its golden is updated.
func TestNoPredictionInRenderedScenarios(t *testing.T) {
	t.Parallel()
	for _, s := range library() {
		name := s.expect.Name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, h := range predictionHits(scenarioNotes(t, name)) {
				t.Errorf("predictive wording: %s", h)
			}
		})
	}
}
