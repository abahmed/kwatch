package scenarios

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/scorecard"
)

// passing is a verdict with one right root.
func passing(name string, confidence float64) verdict {
	e := expectation{Name: name, Root: "deployment/shop/api",
		Tier: "notify", MaxMessages: 2}
	return verdict{expect: e, tier: "notify", messages: 1,
		cases: []scorecard.Case{{Name: name, Correct: true,
			ExpectsRoot: true, Confidence: confidence}}}
}

func TestRatchetListsPassingScenariosAndHowTheirRootWasFound(t *testing.T) {
	t.Parallel()
	failing := passing("broken", 0.9)
	failing.cases[0].Correct = false
	quiet := verdict{expect: expectation{Name: "calm", Quiet: true}}
	text := renderRatchet([]verdict{
		passing("b-found", 0.9), passing("a-self", 0), failing, quiet})
	entries, err := parseRatchet(text)
	if err != nil {
		t.Fatal(err)
	}
	want := []ratchetEntry{{"a-self", "self"}, {"b-found", "found-high"},
		{"calm", "quiet"}}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v, want %v (a failing scenario is not listed)",
			entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d = %v, want %v", i, entries[i], want[i])
		}
	}
}

func TestParseRatchetRejectsLinesWithoutModes(t *testing.T) {
	t.Parallel()
	if _, err := parseRatchet("# note\n\nname-only\n"); err == nil {
		t.Fatal("want an error for a line without modes")
	}
}

func TestRatchetFailuresReportsEachWayToRegress(t *testing.T) {
	t.Parallel()
	broken := passing("broken", 0.9)
	broken.cases[0].Correct = false
	entries := []ratchetEntry{
		{"kept", "found-high"}, {"broken", "found-high"},
		{"gone", "found-high"}, {"fell-back", "found-high"},
		{"improved", "self"}, {"dropped", "found-high"},
	}
	verdicts := []verdict{
		passing("kept", 0.9), broken, passing("fell-back", 0),
		passing("improved", 0.9), passing("dropped", 0.6),
	}
	failures := ratchetFailures(entries, verdicts)
	text := strings.Join(failures, "\n")
	for _, want := range []string{"broken no longer passes",
		"gone is not in the library", "fell-back lost ground",
		"dropped lost ground: found-high is now found-likely"} {
		if !strings.Contains(text, want) {
			t.Errorf("failures = %q, want %q", text, want)
		}
	}
	if len(failures) != 4 {
		t.Errorf("failures = %q, want exactly 4: kept holds and improved "+
			"may find the cause it only fell back to", failures)
	}
}

func TestRatchetGateFailsOnTheFirstRegression(t *testing.T) {
	t.Parallel()
	if gate := ratchetGate(nil); !gate.Pass {
		t.Errorf("gate = %+v, want a pass without failures", gate)
	}
	gate := ratchetGate([]string{"a no longer passes", "b is gone"})
	if gate.Pass || !strings.Contains(gate.Value, "a no longer passes") {
		t.Errorf("gate = %+v, want a miss that names the scenario", gate)
	}
}

func TestLostCauseComparesEachRootInPlace(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		was, now string
		lost     bool
	}{
		"same":               {"found-high", "found-high", false},
		"fell back":          {"found-high", "self", true},
		"lost a level":       {"found-high", "found-likely", true},
		"gained a level":     {"found-likely", "found-high", false},
		"found more":         {"self", "found-likely", false},
		"second root fell":   {"found-high,found-high", "found-high,self", true},
		"fewer roots":        {"found-high,found-high", "found-high", true},
		"quiet stays quiet":  {"quiet", "quiet", false},
		"self stays self":    {"self,unknown", "self,unknown", false},
		"found became quiet": {"found-possible", "quiet", true},
	}
	for name, c := range cases {
		if got := lostCause(c.was, c.now); got != c.lost {
			t.Errorf("%s: lostCause(%q, %q) = %v", name, c.was, c.now, got)
		}
	}
}

// The committed list is read from testdata and must parse; a missing
// file is an error, not an empty ratchet.
func TestReadRatchetParsesTheCommittedList(t *testing.T) {
	t.Parallel()
	entries, err := readRatchet()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("the ratchet list is empty: it holds nothing")
	}
}
