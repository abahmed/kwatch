package scenarios

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// The ratchet pins what already works, one scenario at a time. The
// aggregate gates average over hundreds of scenarios, so one scenario
// that stops passing can hide inside the slack. testdata/ratchet.txt
// lists every labelled scenario that passes, with how its root was found:
//
//	found-high, found-likely, found-possible
//	         the engine stated a cause, at that confidence level
//	self     it rooted at the failing entity and stated no cause
//	unknown  the label expects no cause, and none was stated
//	quiet    the label expects silence
//
// A listed scenario must keep passing, and one listed as "found" must keep
// stating a cause at its level or higher: a cause that falls back to "no
// cause" would otherwise still read as the right root, and one that loses
// a level changes from stated plainly to "likely" in the message. This is
// also what lets the gates see the counterfactual checks, which only move
// confidences. A scenario that is not listed is not held to anything yet.
//
// The list only moves on purpose. After a deliberate change, regenerate it
// and review the diff, since a removed line is a regression accepted:
//
//	go test ./internal/scenarios -run TestScenarioRatchetHolds -update-ratchet
var updateRatchet = flag.Bool("update-ratchet", false,
	"rewrite testdata/ratchet.txt with the scenarios that pass now")

var ratchetPath = filepath.Join("testdata", "ratchet.txt")

const ratchetHeader = "# Labelled scenarios that pass, and how each " +
	"root was found.\n" +
	"# Every line must keep passing: see ratchet_test.go for the rules " +
	"and for\n" +
	"# how to update this file on purpose. One scenario per line, " +
	"sorted.\n"

// ratchetEntry is one line of the list.
type ratchetEntry struct {
	name  string
	modes string
}

// rootModes says how a verdict's roots were found, one word per root.
func rootModes(v verdict) string {
	if v.expect.Quiet {
		return "quiet"
	}
	modes := make([]string, 0, len(v.cases))
	for _, c := range v.cases {
		switch {
		case c.Stated():
			modes = append(modes, "found-"+
				scorecard.ConfidenceLevel(c.Confidence))
		case c.ExpectsRoot:
			modes = append(modes, "self")
		default:
			modes = append(modes, "unknown")
		}
	}
	return strings.Join(modes, ",")
}

// renderRatchet is the list for the scenarios that pass, sorted by name.
func renderRatchet(verdicts []verdict) string {
	var lines []string
	for _, v := range verdicts {
		if len(v.problems()) == 0 {
			lines = append(lines, v.expect.Name+" "+rootModes(v))
		}
	}
	sort.Strings(lines)
	return ratchetHeader + strings.Join(lines, "\n") + "\n"
}

// parseRatchet reads the list: comments and blank lines are skipped, every
// other line is "name modes".
func parseRatchet(text string) ([]ratchetEntry, error) {
	var out []ratchetEntry
	scanner := bufio.NewScanner(strings.NewReader(text))
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, modes, ok := strings.Cut(line, " ")
		if !ok || modes == "" {
			return nil, fmt.Errorf("ratchet line %d: want \"name modes\", "+
				"got %q", n, line)
		}
		out = append(out, ratchetEntry{name: name, modes: modes})
	}
	return out, scanner.Err()
}

// ratchetFailures lists the listed scenarios that no longer hold: they
// are gone, they fail now, or their cause fell back to "no cause".
func ratchetFailures(entries []ratchetEntry, verdicts []verdict) []string {
	byName := map[string]verdict{}
	for _, v := range verdicts {
		byName[v.expect.Name] = v
	}
	var out []string
	for _, entry := range entries {
		v, ok := byName[entry.name]
		switch {
		case !ok:
			out = append(out, entry.name+" is not in the library; remove "+
				"it from the list on purpose")
		case len(v.problems()) > 0:
			out = append(out, entry.name+" no longer passes: "+
				strings.Join(v.problems(), "; "))
		case lostCause(entry.modes, rootModes(v)):
			out = append(out, entry.name+" lost ground: "+entry.modes+
				" is now "+rootModes(v))
		}
	}
	return out
}

// lostCause reports a root that was found with a stated cause and is no
// longer found at that level.
func lostCause(was, now string) bool {
	before, after := strings.Split(was, ","), strings.Split(now, ",")
	for i, mode := range before {
		if foundRank(mode) > 0 &&
			(i >= len(after) || foundRank(after[i]) < foundRank(mode)) {
			return true
		}
	}
	return false
}

// foundRank orders the "found" modes by confidence level; every other
// mode is zero.
func foundRank(mode string) int {
	switch mode {
	case "found-high":
		return 3
	case "found-likely":
		return 2
	case "found-possible":
		return 1
	}
	return 0
}

// ratchetGate is the ratchet as a scorecard gate.
func ratchetGate(failures []string) scorecard.Gate {
	gate := scorecard.AtMost(
		"Scenarios that passed and no longer do (ratchet)",
		float64(len(failures)), 0, "")
	if len(failures) > 0 {
		shown := failures[:min(3, len(failures))]
		gate.Value += ": " + strings.Join(shown, "; ")
	}
	return gate
}

// readRatchet loads the committed list. A missing file is an error: a
// ratchet that is silently absent holds nothing.
func readRatchet() ([]ratchetEntry, error) {
	data, err := os.ReadFile(ratchetPath)
	if err != nil {
		return nil, fmt.Errorf("%w; create it with: go test "+
			"./internal/scenarios -run TestScenarioRatchetHolds "+
			"-update-ratchet", err)
	}
	return parseRatchet(string(data))
}

// labelledVerdicts replays and judges every labelled scenario.
func labelledVerdicts(t testing.TB) []verdict {
	t.Helper()
	var out []verdict
	for _, s := range library() {
		log, e, result := replayScenario(t, labelledDir, s.expect.Name)
		out = append(out, judge(e, log, result))
	}
	return out
}

// freshVerdicts replays and judges every labelled scenario from its
// generator, not its committed log, so the list written by -update-ratchet
// matches the logs a regeneration writes.
func freshVerdicts(t testing.TB) []verdict {
	t.Helper()
	var out []verdict
	for _, s := range library() {
		log := s.generate(scenarioStart, "")
		opts := s.expect.options(log.Start)
		result, err := replay.Run(context.Background(), log,
			withSync(newDependencies(s.expect.hiddenKinds()...), opts), opts)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, judge(s.expect, log, result))
	}
	return out
}

// TestScenarioRatchetHolds fails when a scenario on the list stopped
// passing. With -update-ratchet it rewrites the list instead.
func TestScenarioRatchetHolds(t *testing.T) {
	if *updateRatchet {
		text := renderRatchet(freshVerdicts(t))
		if err := os.WriteFile(ratchetPath, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	entries, err := readRatchet()
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range ratchetFailures(entries, labelledVerdicts(t)) {
		t.Error(failure)
	}
}
