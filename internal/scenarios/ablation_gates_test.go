package scenarios

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// The gates are only worth having if they can see a root-cause
// regression. This is the mutation check: it switches a part of the
// engine off through explain.Ablation, replays a few scenarios that
// depend on that part, and asserts that at least one gate fails, while
// the same scenarios pass every gate with the engine intact. A part of the
// engine no scenario depends on shows up here as a test that cannot fail.
var ablations = []struct {
	name      string
	ablation  explain.Ablation
	scenarios []string
}{
	{"the specificity scorer", explain.Ablation{
		Scorers: []string{"specificity"}},
		[]string{"missing-secret"}},
	{"every veto", explain.Ablation{Vetoes: true},
		[]string{"bad-config-value-change", "nodes-lost-same-zone"}},
	// The counterfactual checks only move confidences, so the ratchet's
	// confidence levels are what sees them. One scenario pins them so far.
	{"the counterfactual checks", explain.Ablation{
		Scorers: explain.CounterfactualScorers()},
		[]string{"adv-hpa-max-throttled"}},
}

func TestScorecardGatesSeeRootCauseRegressions(t *testing.T) {
	t.Parallel()
	for _, a := range ablations {
		t.Run(a.name, func(t *testing.T) {
			t.Parallel()
			intact := ablatedVerdicts(t, a.scenarios, explain.Ablation{})
			baseline, err := parseRatchet(renderRatchet(intact))
			if err != nil {
				t.Fatal(err)
			}
			for _, gate := range scorecard.Failed(
				regressionGates(intact, baseline)) {
				t.Fatalf("with the engine intact, gate %q fails: %s; the "+
					"scenarios cannot serve as a control", gate.Name,
					gate.Value)
			}
			broken := ablatedVerdicts(t, a.scenarios, a.ablation)
			if len(scorecard.Failed(regressionGates(broken, baseline))) == 0 {
				t.Errorf("no gate fails without %s: the scorecard cannot "+
					"see that part of the engine regress", a.name)
			}
		})
	}
}

// regressionGates are the gates that count scenarios, applied to a few.
// The percentage gates mean nothing over so few cases.
func regressionGates(
	verdicts []verdict, baseline []ratchetEntry,
) []scorecard.Gate {
	var cases []scorecard.Case
	for _, v := range verdicts {
		cases = append(cases, v.cases...)
	}
	gates := labelGates(verdicts)
	gates = append(gates, rootGates(verdicts, scorecard.ScoreCases(cases))...)
	return append(gates, ratchetGate(ratchetFailures(baseline, verdicts)))
}

// ablatedVerdicts replays the named scenarios from their generators with
// part of the scoring switched off, and judges them.
func ablatedVerdicts(
	t *testing.T, names []string, a explain.Ablation,
) []verdict {
	t.Helper()
	var out []verdict
	for _, name := range names {
		s := scenarioNamed(t, name)
		log := s.generate(scenarioStart, "")
		opts := s.expect.options(log.Start)
		result, err := replay.Run(context.Background(), log,
			withSync(newAblatedDependencies(a), opts), opts)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, judge(s.expect, log, result))
	}
	return out
}

func scenarioNamed(t *testing.T, name string) scenario {
	t.Helper()
	for _, s := range library() {
		if s.expect.Name == name {
			return s
		}
	}
	t.Fatalf("scenario %s is not in the library", name)
	return scenario{}
}
