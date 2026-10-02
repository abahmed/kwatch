package scenarios

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// Gate switches. The report always prints; make alert-quality-gate,
// part of make verify and the alert-quality workflow, fails on any miss.
var (
	enforceGates = flag.Bool("gate", os.Getenv("KWATCH_SCORECARD_GATE") == "1",
		"fail TestScorecardGates when any gate misses its target")
	reportPath = flag.String("report", os.Getenv("KWATCH_SCORECARD_REPORT"),
		"write the scorecard report as Markdown to this file")
)

// card is everything the gates are computed from.
type card struct {
	verdicts []verdict
	// cases are the labelled root cases the accuracy is scored from.
	cases    []scorecard.Case
	accuracy scorecard.Accuracy
	heldout  heldoutResult
	// firstMessages are the labelled scenarios that interrupted people.
	firstMessages []firstMessage
	noise         scorecard.Report
	staging       stagingResult
	storms        []stormResult
	multi         multiStormResult
}

// TestScorecardGates replays every labelled scenario, the storms and the
// staging day, and reports each alert-quality gate of
// docs/production-goals.md with its target, value and verdict.
//
//	make alert-quality       # report only
//	make alert-quality-gate  # fail on any miss
func TestScorecardGates(t *testing.T) {
	c := measure(t)
	gates := buildGates(c)
	report := renderReport(c, gates)
	t.Log("\n" + report)
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, []byte(report), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, gate := range scorecard.Failed(gates) {
		message := fmt.Sprintf("gate missed: %s is %s, target %s",
			gate.Name, gate.Value, gate.Target)
		if *enforceGates {
			t.Error(message)
		} else {
			t.Log(message)
		}
	}
}

func measure(t *testing.T) card {
	t.Helper()
	var c card
	var entries []audit.Entry
	var cases []scorecard.Case
	for _, s := range library() {
		log, e := loadScenario(t, s.expect.Name)
		result := replayLog(t, log, e.options(log.Start))
		v := judge(e, result)
		c.verdicts = append(c.verdicts, v)
		cases = append(cases, v.cases...)
		entries = append(entries, auditEntries(e.Name, result)...)
		// A cold start measures the sync, not detection.
		if m, ok := timeToFirstMessage(e.Name, log, result); ok &&
			e.SyncAfter == 0 {
			c.firstMessages = append(c.firstMessages, m)
		}
	}
	c.cases = cases
	c.accuracy = scorecard.ScoreCases(cases)
	c.heldout = measureHeldOut(t)
	c.staging = runStagingDay(t)
	entries = append(entries, c.staging.entries...)
	c.noise = scorecard.Score(sortedEntries(entries))
	c.storms = runStorms(t)
	c.multi = runMultiCauseStorm(t)
	return c
}

func buildGates(c card) []scorecard.Gate {
	gates := []scorecard.Gate{
		scorecard.AtLeast("Correct root cause (labelled)",
			c.accuracy.CorrectPercent(),
			scorecard.GoalCorrectRootPercent, "%"),
	}
	gates = append(gates, heldoutGates(c.heldout)...)
	gates = append(gates, wrongHighGate(c.cases, c.heldout.cases))
	gates = append(gates, calibrationGates(c.accuracy)...)
	gates = append(gates,
		scorecard.AtMost("Messages per incident (p95)",
			float64(c.noise.PerIncidentP95),
			scorecard.GoalMessagesPerIncidentP95, ""),
		scorecard.AtMost("Messages per incident (most)",
			float64(c.noise.MaxPerIncident),
			scorecard.GoalMessagesPerIncident, ""),
	)
	gates = append(gates, firstMessageGates(c.firstMessages)...)
	gates = append(gates,
		scorecard.AtMost("Notifications from non-events (staging day)",
			float64(c.staging.nonEvents),
			scorecard.GoalNonEventNotifications, ""),
		scorecard.AtMost("Notifications per hour (staging day peak)",
			float64(c.staging.peakPerHour), scorecard.GoalPeakPerHour,
			"/h"),
		scorecard.AtMost("Unchanged updates", c.noise.UnchangedUpdatePercent(),
			scorecard.GoalUnchangedPercent, "%"),
		scorecard.AtMost("Re-created incidents", c.noise.RecreatedPercent(),
			scorecard.GoalRecreatedPercent, "%"),
		scorecard.AtMost("Repeated recoveries",
			float64(c.noise.RepeatedResolves),
			scorecard.GoalRepeatedResolves, ""),
	)
	for _, storm := range c.storms {
		gates = append(gates, scorecard.AtMost(
			"Storm messages in 2 minutes ("+storm.name+")",
			float64(storm.peak), scorecard.GoalStormMessages, ""))
	}
	gates = append(gates, multiStormGates(c.multi)...)
	return append(gates, labelGates(c.verdicts)...)
}

// wrongHighGate bounds how often a plainly stated cause is wrong, over
// the labelled and the held-out cases together: the unseen set is where
// a confident wrong answer hurts most. With fewer than
// GoalWrongHighMinCases high-confidence cases it is reported but not
// gated: one case would move it by more than five points.
func wrongHighGate(labelled, heldout []scorecard.Case) scorecard.Gate {
	all := append(append([]scorecard.Case(nil), labelled...), heldout...)
	a := scorecard.ScoreCases(all)
	gate := scorecard.AtMost("Wrong high-confidence root",
		a.WrongHighPercent(), scorecard.GoalWrongHighPercent, "%")
	gate.Target += fmt.Sprintf(" (at least %d cases)",
		scorecard.GoalWrongHighMinCases)
	gate.Value += fmt.Sprintf(
		" (%d of %d high-confidence cases, labelled and held-out)",
		a.WrongHigh, a.HighCases())
	if a.HighCases() < scorecard.GoalWrongHighMinCases {
		gate.Value = "not gated: " + gate.Value
		gate.Pass = true
	}
	return gate
}

// calibrationBand is the observed accuracy a confidence level must
// show, in percent. It is two-sided: a level right less often than it
// claims is overconfident, and one right far more often is
// underconfident, so it hides certainty readers could rely on.
type calibrationBand struct {
	low, high float64
}

// calibrationBands are the bands of the levels that make a claim.
var calibrationBands = map[string]calibrationBand{
	scorecard.LevelHigh:   {low: 80, high: 100},
	scorecard.LevelLikely: {low: 50, high: 90},
}

// minCalibrationCases is the fewest cases a level needs before its band
// is gated. Below it, one case moves the accuracy by more than ten
// points, which says more about the sample than about calibration.
const minCalibrationCases = 10

// calibrationGates checks each confidence level's observed accuracy is
// within its band. A level with too few cases is reported as not gated.
func calibrationGates(a scorecard.Accuracy) []scorecard.Gate {
	var gates []scorecard.Gate
	for _, level := range a.Levels {
		band, ok := calibrationBands[level.Name]
		if !ok {
			continue
		}
		gates = append(gates, calibrationGate(level, band))
	}
	return gates
}

func calibrationGate(
	level scorecard.Level, band calibrationBand,
) scorecard.Gate {
	accuracy := level.AccuracyPercent()
	gate := scorecard.Gate{
		Name: "Calibration: " + level.Name + " confidence",
		Target: fmt.Sprintf("%g-%g%% (at least %d cases)",
			band.low, band.high, minCalibrationCases),
		Value: fmt.Sprintf("%.1f%% (%d of %d right)", accuracy,
			level.Correct, level.Cases),
		Pass: accuracy >= band.low && accuracy <= band.high,
	}
	if level.Cases < minCalibrationCases {
		gate.Value = fmt.Sprintf("not gated: %d of %d cases needed "+
			"(%d of %d right)", level.Cases, minCalibrationCases,
			level.Correct, level.Cases)
		gate.Pass = true
	}
	return gate
}

// labelGates check the per-scenario labels: blame, quiet and budget.
func labelGates(verdicts []verdict) []scorecard.Gate {
	blamed, loud, over := 0, 0, 0
	for _, v := range verdicts {
		if len(v.blamed) > 0 {
			blamed++
		}
		if v.loud() {
			loud++
		}
		if v.overBudget() {
			over++
		}
	}
	return []scorecard.Gate{
		scorecard.AtMost("Scenarios blaming a must-not-blame entity",
			float64(blamed), 0, ""),
		scorecard.AtMost("Quiet scenarios that alerted", float64(loud), 0, ""),
		scorecard.AtMost("Scenarios over their message budget",
			float64(over), 0, ""),
	}
}

// sortedEntries orders entries by time; the entries of one incident keep
// their order.
func sortedEntries(entries []audit.Entry) []audit.Entry {
	out := append([]audit.Entry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}
