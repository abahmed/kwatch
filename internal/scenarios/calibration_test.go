package scenarios

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
	"github.com/abahmed/kwatch/internal/scorecard"
)

func TestCalibrationGateIsTwoSided(t *testing.T) {
	band := calibrationBands[scorecard.LevelLikely]
	cases := map[string]struct {
		correct, total int
		pass           bool
		value          string
	}{
		"inside the band":      {14, 20, true, "70.0%"},
		"at the ceiling":       {19, 20, true, "95.0%"},
		"overconfident":        {8, 20, false, "40.0%"},
		"underconfident":       {20, 20, false, "100.0%"},
		"too few cases to say": {6, 6, true, "not gated"},
		"no cases":             {0, 0, true, "not gated"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			gate := calibrationGate(scorecard.Level{
				Name: scorecard.LevelLikely, Cases: c.total,
				Correct: c.correct,
			}, band)
			if gate.Pass != c.pass ||
				!strings.HasPrefix(gate.Value, c.value) {
				t.Fatalf("gate = %+v", gate)
			}
		})
	}
}

func TestCalibrationGatesCoverEveryClaimingLevel(t *testing.T) {
	gates := calibrationGates(scorecard.ScoreCases(nil))
	if len(gates) != len(calibrationBands) {
		t.Fatalf("gates = %+v, want one per band", gates)
	}
}

// TestConfidenceThresholdsAgree keeps the three copies of the calibrated
// thresholds equal: the engine's, the one messages are worded by and
// the one the scorecard judges by. Packages below the engine cannot
// import it, so each keeps its own constant.
func TestConfidenceThresholdsAgree(t *testing.T) {
	highs := []float64{explain.ConfidenceHigh, rootcause.High,
		scorecard.HighConfidence}
	likelies := []float64{explain.ConfidenceLikely, rootcause.Likely,
		scorecard.LikelyConfidence}
	for _, values := range [][]float64{highs, likelies} {
		for _, v := range values[1:] {
			if v != values[0] {
				t.Fatalf("thresholds differ: %v", values)
			}
		}
	}
}

// casesAt builds n labelled cases scored at score, right of them right.
func casesAt(score float64, n, right int) []scorecard.Case {
	out := make([]scorecard.Case, n)
	for i := range out {
		out[i] = scorecard.Case{Confidence: score, Correct: i < right}
	}
	return out
}

// TestBoundaryGateHoldsTheCommittedBoundary checks the boundary gate
// passes while the labelled cases put the boundary within the drift
// allowed around the committed one, and fails outside it either way.
func TestBoundaryGateHoldsTheCommittedBoundary(t *testing.T) {
	cases := map[string]struct {
		cases []scorecard.Case
		pass  bool
		value string
	}{
		"at the boundary": {casesAt(0.72, 12, 12), true, "0.70"},
		"one bucket above": {append(casesAt(0.62, 12, 6),
			casesAt(0.77, 12, 12)...), true, "0.75"},
		"too far above": {append(casesAt(0.72, 12, 6),
			casesAt(0.82, 12, 12)...), false, "0.80"},
		"at the floor":  {casesAt(0.57, 12, 12), true, "0.55"},
		"too far below": {casesAt(0.52, 12, 12), false, "0.50"},
		"nothing qualifies": {casesAt(0.9, 12, 3), false,
			"no score range"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			gate := boundaryGate(c.cases, 0.70)
			if gate.Pass != c.pass ||
				!strings.HasPrefix(gate.Value, c.value) {
				t.Fatalf("gate = %+v", gate)
			}
		})
	}
}

// TestHighPrecisionGate checks the cases scored at or above the
// boundary must be right at least calibrationHighPrecision percent of
// the time, once there are enough of them.
func TestHighPrecisionGate(t *testing.T) {
	below := casesAt(0.6, 20, 0)
	for name, c := range map[string]struct {
		cases []scorecard.Case
		pass  bool
	}{
		"all right":   {append(below, casesAt(0.8, 20, 20)...), true},
		"at target":   {casesAt(0.7, 25, 23), true},
		"below":       {casesAt(0.75, 20, 18), false},
		"too few":     {casesAt(0.9, 5, 0), true},
		"only below":  {below, true},
		"edge scored": {casesAt(0.7, 10, 9), false},
	} {
		t.Run(name, func(t *testing.T) {
			gate := highPrecisionGate(c.cases, 0.70)
			if gate.Pass != c.pass {
				t.Fatalf("gate = %+v", gate)
			}
		})
	}
}
