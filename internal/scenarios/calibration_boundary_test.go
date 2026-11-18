package scenarios

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/scorecard"
)

// The high-confidence boundary (rootcause.High) is a constant chosen
// once by scorecard.CalibratedHigh from the labelled cases. These gates
// fail the build when the labelled cases no longer support it, instead
// of only printing the boundary they give (calibrationLine).
const (
	// calibrationDriftAbove is how far above rootcause.High the
	// computed boundary may sit. Higher, and causes scored between
	// the two are stated plainly while the labelled cases show that
	// range right less often than a plain statement promises: kwatch
	// would be overconfident, the harmful direction, so the margin is
	// one calibration bucket (scorecard.CalibrationBucket).
	calibrationDriftAbove = 0.05
	// calibrationDriftBelow is how far below rootcause.High the
	// computed boundary may sit. Lower means causes worded as likely
	// are right as often as plain ones: underconfidence, which hides
	// certainty but states nothing wrong, and which the likely band's
	// ceiling also bounds. It is three buckets because the boundary
	// already stands at 0.55: 19 labelled cases scored in
	// [0.55, 0.60) are all right. A further bucket down fails.
	calibrationDriftBelow = 0.15
	// calibrationHighPrecision is the accuracy, in percent, that the
	// labelled cases scored at or above rootcause.High must keep: the
	// accuracy the boundary was calibrated at (rootcause/confidence.go).
	calibrationHighPrecision = 92
	// boundaryEpsilon absorbs the rounding of the bucket arithmetic.
	boundaryEpsilon = 1e-9
)

// boundaryGates are the calibration gates of the committed boundary
// high, over the labelled cases.
func boundaryGates(cases []scorecard.Case, high float64) []scorecard.Gate {
	return []scorecard.Gate{boundaryGate(cases, high),
		highPrecisionGate(cases, high)}
}

// boundaryGate checks that the boundary the labelled cases give stays
// within the drift allowed around high. No qualifying range at all
// fails: nothing could then be stated plainly.
func boundaryGate(cases []scorecard.Case, high float64) scorecard.Gate {
	low, top := high-calibrationDriftBelow, high+calibrationDriftAbove
	gate := scorecard.Gate{
		Name: "Calibration: high-confidence boundary",
		Target: fmt.Sprintf("%.2f-%.2f (committed %.2f)", low, top,
			high),
	}
	computed, ok := scorecard.CalibratedHigh(cases)
	if !ok {
		gate.Value = "no score range qualifies"
		return gate
	}
	gate.Value = fmt.Sprintf("%.2f", computed)
	gate.Pass = computed >= low-boundaryEpsilon &&
		computed <= top+boundaryEpsilon
	return gate
}

// highPrecisionGate checks that the labelled cases scored at or above
// high are right at least calibrationHighPrecision percent of the
// time. With fewer than minCalibrationCases it is reported, not gated.
func highPrecisionGate(
	cases []scorecard.Case, high float64,
) scorecard.Gate {
	var at scorecard.Level
	for _, c := range cases {
		if c.Confidence+boundaryEpsilon < high {
			continue
		}
		at.Cases++
		if c.Correct {
			at.Correct++
		}
	}
	accuracy := at.AccuracyPercent()
	gate := scorecard.Gate{
		Name: "Calibration: precision at the high boundary",
		Target: fmt.Sprintf(">= %d%% (at least %d cases)",
			calibrationHighPrecision, minCalibrationCases),
		Value: fmt.Sprintf("%.1f%% (%d of %d right)", accuracy,
			at.Correct, at.Cases),
		Pass: accuracy >= calibrationHighPrecision,
	}
	if at.Cases < minCalibrationCases {
		gate.Value = "not gated: " + gate.Value
		gate.Pass = true
	}
	return gate
}
