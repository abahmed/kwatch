package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

func TestRootGatesCountWrongStatedRootsAndWrongFirstRoots(t *testing.T) {
	t.Parallel()
	cases := []scorecard.Case{
		{Correct: true, Confidence: 0.9, ExpectsRoot: true},
		{Correct: true, Confidence: 0.9, ExpectsRoot: true},
	}
	clean := []verdict{{expect: expectation{Name: "a"}},
		{expect: expectation{Name: "b"}}}
	for _, gate := range rootGates(clean, scorecard.ScoreCases(cases)) {
		if !gate.Pass {
			t.Errorf("gate %q failed on clean verdicts: %+v", gate.Name, gate)
		}
	}
	cases = append(cases, scorecard.Case{Confidence: 0.6, ExpectsRoot: true})
	dirty := append(clean, verdict{expect: expectation{Name: "c"},
		firstWrong: []string{"node//n1"}, announcedWrong: []string{"node//n1"}})
	dirty = append(dirty, verdict{
		expect:     expectation{Name: "d", RevisesRoot: true},
		firstWrong: []string{"node//n2"}})
	gates := rootGates(dirty, scorecard.ScoreCases(cases))
	if gates[0].Pass || gates[0].Value != "1" {
		t.Errorf("wrong-stated gate = %+v, want 1 and a miss", gates[0])
	}
	if gates[1].Pass || gates[1].Value != "1" {
		t.Errorf("first-root gate = %+v, want 1: the labelled revision "+
			"opts out", gates[1])
	}
}

func TestFoundLineSaysHowRootsWereFound(t *testing.T) {
	t.Parallel()
	accuracy := scorecard.ScoreCases([]scorecard.Case{
		{Correct: true, Confidence: 0.9, ExpectsRoot: true},
		{Correct: true, ExpectsRoot: true},
		{Correct: true},
	})
	line := foundLine(accuracy, []verdict{
		{expect: expectation{RevisesRoot: true},
			announcedWrong: []string{"x"}},
	})
	for _, want := range []string{"1 with a stated cause",
		"1 by falling back", "1 by stating no cause",
		"1 scenarios, 1 of them labelled"} {
		if !strings.Contains(line, want) {
			t.Errorf("line = %q, want %q", line, want)
		}
	}
}

func TestTimeToFirstRootMeasuresTheFirstMessageWithTheRoot(t *testing.T) {
	t.Parallel()
	api := entityID("deployment", "shop", "api")
	node := entityID("node", "", "n1")
	log := replay.Log{Start: judgeStart}
	wrongFirst := causedIncident("inc-1", node, 0.8)
	wrongFirst.Opened = judgeAt(1)
	right := causedIncident("inc-1", api, 0.8)
	right.Opened = judgeAt(1)
	result := replay.Result{
		Decisions: []incident.Decision{
			announceDecision(wrongFirst),
			{Action: incident.Update, Incident: right},
		},
		Times: []time.Time{judgeAt(3), judgeAt(8)},
	}
	e := expectation{Name: "revised", Root: "deployment/shop/api"}
	got, ok := timeToFirstRoot(e, log, result)
	if !ok || got.delay != 7*time.Minute {
		t.Fatalf("first root = %+v, %v, want 7m after the failure began",
			got, ok)
	}
	e.Root = "deployment/shop/other"
	if _, ok := timeToFirstRoot(e, log, result); ok {
		t.Error("a root no message stated must not be measured")
	}
	if _, ok := timeToFirstRoot(expectation{Quiet: true}, log, result); ok {
		t.Error("a quiet scenario has no root to time")
	}
}

func TestFirstRootLineNamesTheSlowestAndTheMissing(t *testing.T) {
	t.Parallel()
	line := firstRootLine([]firstRoot{{"fast", time.Minute},
		{"slow", 9 * time.Minute}}, []string{"never-found"})
	for _, want := range []string{"max 9m0s", "slowest: slow 9m0s, fast 1m0s",
		"Never stated: never-found"} {
		if !strings.Contains(line, want) {
			t.Errorf("line = %q, want %q", line, want)
		}
	}
	if got := firstRootLine(nil, nil); !strings.Contains(got, "no cases") {
		t.Errorf("empty line = %q", got)
	}
}

// Advisory states are non-events too: they may not cause any message.
func TestNonEventCasesIncludeAdvisoryStates(t *testing.T) {
	t.Parallel()
	have := map[string]bool{}
	for _, c := range nonEventCases() {
		have[c.name] = true
	}
	for _, want := range []string{"quota used exactly, nothing refused",
		"healthy pods throttled on CPU",
		"budget allowing no disruption, no drain"} {
		if !have[want] {
			t.Errorf("non-event %q is missing", want)
		}
	}
}

func TestDependenciesLosingSightStaySynced(t *testing.T) {
	t.Parallel()
	deps := newDependenciesLosingSight(inventory.Kind("secret"))
	if !deps.Synced("secret") {
		t.Error("a kind that was visible and then lost stays synced")
	}
}
