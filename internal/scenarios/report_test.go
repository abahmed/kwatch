package scenarios

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/scorecard"
)

func renderReport(c card, gates []scorecard.Gate) string {
	var b strings.Builder
	b.WriteString("## Gates\n\n")
	b.WriteString(scorecard.GateTable(gates))
	fmt.Fprintf(&b, "\n%d of %d gates pass. %d labelled root cases in %d "+
		"scenarios; first-incident tier matches in %d of %d scenarios.\n",
		len(gates)-len(scorecard.Failed(gates)), len(gates),
		c.accuracy.Cases, len(c.verdicts), tierMatches(c.verdicts),
		len(c.verdicts))
	fmt.Fprintf(&b, "Staging day: %d notifications in %s (%.1f/h mean, "+
		"peak hour %d); %d from non-events, %d of them notify or page.\n",
		c.staging.messages, c.staging.window.Round(time.Minute),
		c.staging.meanPerHour(), c.staging.peakPerHour,
		c.staging.nonEvents, c.staging.loudNonEvents)
	for _, storm := range c.storms {
		fmt.Fprintf(&b, "Storm %s: %d pods, %d messages in total, %d in "+
			"the loudest 2 minutes.\n", storm.name, storm.pods,
			storm.messages, storm.peak)
	}
	fmt.Fprintf(&b, "Storm %s: %d pods from %d causes, %d of them "+
		"reported, %d messages in total, %d in the loudest 2 minutes.\n",
		c.multi.name, c.multi.pods, c.multi.causes, c.multi.found,
		c.multi.messages, c.multi.peak)
	b.WriteString(firstMessageLine(c.firstMessages))
	b.WriteString(calibrationLine(c.cases))
	b.WriteString("\n## Scenarios\n\n")
	b.WriteString(scenarioTable(c.verdicts))
	b.WriteString("\n## Held-out scenarios\n\n")
	fmt.Fprintf(&b, "%d held-out root cases in %d scenarios, %d right. "+
		"Never used to tune the engine.\n\n", c.heldout.accuracy.Cases,
		len(c.heldout.verdicts), c.heldout.accuracy.Correct)
	b.WriteString(scenarioTable(c.heldout.verdicts))
	return b.String()
}

// calibrationLine prints the high-confidence boundary the calibration
// method gives for the labelled cases next to the committed one, so a
// labelled set that has moved on is noticed.
func calibrationLine(cases []scorecard.Case) string {
	computed, ok := scorecard.CalibratedHigh(cases)
	if !ok {
		return fmt.Sprintf("Calibration: no score range qualifies for "+
			"high confidence (committed %.2f).\n",
			scorecard.HighConfidence)
	}
	return fmt.Sprintf("Calibration: the labelled cases put high "+
		"confidence at %.2f (committed %.2f).\n", computed,
		scorecard.HighConfidence)
}

// firstMessageLine names the slowest first messages, so a miss of the
// time-to-first-message gate points at its scenarios.
func firstMessageLine(measured []firstMessage) string {
	sorted := append([]firstMessage(nil), measured...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].delay > sorted[j].delay
	})
	parts := make([]string, 0, 5)
	for _, m := range sorted[:min(5, len(sorted))] {
		parts = append(parts, fmt.Sprintf("%s %s %s", m.scenario,
			m.tier, m.delay))
	}
	return "Slowest first messages: " + strings.Join(parts, ", ") + ".\n"
}

func tierMatches(verdicts []verdict) int {
	n := 0
	for _, v := range verdicts {
		if v.tierMatches() {
			n++
		}
	}
	return n
}

func scenarioTable(verdicts []verdict) string {
	var b strings.Builder
	b.WriteString("| Scenario | Expected root | Actual root | Confidence " +
		"| Tier (expected/actual) | Messages (max) | Result |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, v := range verdicts {
		expected, actual, confidence := "quiet", "-", "-"
		if len(v.cases) > 0 {
			expected, actual = joinCases(v.cases)
			confidence = scorecard.ConfidenceLevel(v.cases[0].Confidence)
			if v.cases[0].Confidence > 0 {
				confidence += fmt.Sprintf(" %.2f", v.cases[0].Confidence)
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s/%s | %d (%d) | %s |\n",
			v.expect.Name, expected, actual, confidence,
			orDash(v.expect.Tier), orDash(v.tier), v.messages,
			v.expect.MaxMessages, outcome(v))
	}
	return b.String()
}

func joinCases(cases []scorecard.Case) (string, string) {
	var expected, actual []string
	for _, c := range cases {
		expected = append(expected, "`"+c.Expected+"`")
		actual = append(actual, "`"+c.Actual+"`")
	}
	return strings.Join(expected, ", "), strings.Join(actual, ", ")
}

// outcome summarises every way a scenario missed its label.
func outcome(v verdict) string {
	var misses []string
	for _, c := range v.cases {
		if !c.Correct {
			misses = append(misses, "wrong root")
			break
		}
	}
	if len(v.blamed) > 0 {
		misses = append(misses, "blamed "+strings.Join(v.blamed, ", "))
	}
	if v.loud() {
		misses = append(misses, "not quiet")
	}
	if v.overBudget() {
		misses = append(misses, "over budget")
	}
	if !v.tierMatches() {
		misses = append(misses, "tier")
	}
	if len(misses) == 0 {
		return "pass"
	}
	return strings.Join(misses, "; ")
}

func orDash(text string) string {
	if text == "" {
		return "-"
	}
	return text
}
