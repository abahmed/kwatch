package scorecard

import "math"

// Calibration method. The confidence levels are bucket boundaries over
// the engine's score, chosen once from the labelled scenarios and then
// committed as constants (HighConfidence and its copies in rootcause
// and explain). The report prints what the method gives for the current
// labelled set, so a drift between it and the committed value is seen.
//
// The method is isotonic in spirit: a score range may be stated plainly
// only if it, and everything scored above it, is right at least as often
// as the high band demands, and only a range with enough cases to judge
// may move the boundary at all.
const (
	// CalibrationBucket is the width of the score ranges compared. It
	// matches the step of the engine's priors and weights, so the
	// cases one row produces fall into one range.
	CalibrationBucket = 0.05
	// CalibrationMinCases is the fewest cases a range needs before it
	// may move the high boundary: below it, one case moves its
	// accuracy by more than ten points.
	CalibrationMinCases = 10
	// CalibrationHighAccuracy is the least accuracy, in percent, a
	// range stated plainly must show: the low end of the high band.
	CalibrationHighAccuracy = 80
)

// CalibratedHigh returns the lowest score boundary at which a stated
// cause can be worded plainly, from cases: the lower edge of the lowest
// range that has at least CalibrationMinCases cases, is right at least
// CalibrationHighAccuracy percent of the time, and whose cases together
// with every case scored above it are too. Ranges below LikelyConfidence
// are never considered. ok is false when no range qualifies.
func CalibratedHigh(cases []Case) (boundary float64, ok bool) {
	counts, lowest, highest := calibrationBuckets(cases)
	var above Level
	found := -1
	for bucket := highest; bucket >= lowest; bucket-- {
		level := counts[bucket]
		if level == nil {
			continue
		}
		above.Cases += level.Cases
		above.Correct += level.Correct
		if above.AccuracyPercent() < CalibrationHighAccuracy {
			break
		}
		if level.Cases >= CalibrationMinCases &&
			level.AccuracyPercent() >= CalibrationHighAccuracy {
			found = bucket
		}
	}
	if found < 0 {
		return 0, false
	}
	return math.Round(float64(found)*CalibrationBucket*100) / 100, true
}

// calibrationBuckets counts the cases at or above LikelyConfidence per
// score range, numbered by the range's lower edge in CalibrationBucket
// steps, and returns the lowest and highest numbers used.
func calibrationBuckets(cases []Case) (
	counts map[int]*Level, lowest, highest int,
) {
	counts = map[int]*Level{}
	lowest, highest = math.MaxInt, math.MinInt
	for _, c := range cases {
		if c.Confidence < LikelyConfidence {
			continue
		}
		// The epsilon keeps 0.7 in the 0.70 range despite rounding.
		bucket := int(math.Floor(c.Confidence/CalibrationBucket + 1e-9))
		if counts[bucket] == nil {
			counts[bucket] = &Level{}
		}
		counts[bucket].Cases++
		if c.Correct {
			counts[bucket].Correct++
		}
		lowest, highest = min(lowest, bucket), max(highest, bucket)
	}
	return counts, lowest, highest
}
