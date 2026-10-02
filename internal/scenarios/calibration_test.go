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
