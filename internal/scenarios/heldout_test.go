package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/internal/scorecard"
)

// The held-out set measures how the engine does on failures it was not
// built against. Its rule, also stated in SCORECARD.md and
// docs/production-goals.md:
//
//   - Each scenario is written from a description of a Kubernetes
//     failure: what the cluster shows and what a person who knows
//     Kubernetes would name as the root. The label is fixed before the
//     scenario is first replayed.
//   - A held-out miss is reported, never fixed by changing rules,
//     weights, tiers or the label to fit these files. A fix for a missed
//     failure needs its own labelled scenario first, and the held-out
//     scenario stays as it is.
//   - Held-out scenarios are scored and reported on their own. They are
//     not part of the labelled accuracy, the calibration bands or the
//     staging day. They do count in the wrong-high gate: a confidently
//     wrong answer on an unseen failure is the one that matters most.
//   - Scenarios looked at to find the gap behind a miss are seen: they
//     move into the labelled set under their own names, and fresh
//     held-out scenarios for other failures replace them, labelled
//     before their first replay (SCORECARD.md, held-out rotation).
//
// The committed files live in testdata/heldout and are regenerated with
// the labelled ones (TestScenarioFixtures -update).
func heldoutLibrary() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		heldoutWorkloadScenarios(), heldoutClusterScenarios(),
		heldoutBatchScenarios(), heldoutStormScenarios(),
		heldoutPlacementScenarios(), heldoutStartupScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// heldoutResult is the replay of every held-out scenario.
type heldoutResult struct {
	verdicts []verdict
	// cases are the held-out root cases, kept for the wrong-high gate.
	cases    []scorecard.Case
	accuracy scorecard.Accuracy
}

func measureHeldOut(t *testing.T) heldoutResult {
	t.Helper()
	var r heldoutResult
	var cases []scorecard.Case
	for _, s := range heldoutLibrary() {
		_, e, result := replayScenario(t, heldoutDir, s.expect.Name)
		v := judge(e, result)
		r.verdicts = append(r.verdicts, v)
		cases = append(cases, v.cases...)
	}
	r.cases = cases
	r.accuracy = scorecard.ScoreCases(cases)
	return r
}

// heldoutGates is the one held-out gate: correct root. Blame, tier and
// budget misses are listed in the held-out table but not gated, so the
// set stays a measure of accuracy rather than a second labelled suite.
func heldoutGates(r heldoutResult) []scorecard.Gate {
	return []scorecard.Gate{scorecard.AtLeast(
		"Correct root cause (held-out)", r.accuracy.CorrectPercent(),
		scorecard.GoalHeldOutCorrectPercent, "%")}
}

// TestHeldOutSetIsSeparate keeps the two sets apart: no held-out name
// may also be labelled, and the held-out set is big enough to score.
func TestHeldOutSetIsSeparate(t *testing.T) {
	labelled := map[string]bool{}
	for _, s := range library() {
		labelled[s.expect.Name] = true
	}
	held := heldoutLibrary()
	if len(held) < 12 {
		t.Fatalf("held-out set has %d scenarios, want at least 12",
			len(held))
	}
	for _, s := range held {
		if labelled[s.expect.Name] {
			t.Errorf("%s is both labelled and held out", s.expect.Name)
		}
	}
}
