package scorecard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfidenceLevelNamesEachBand(t *testing.T) {
	cases := map[float64]string{
		0.9: LevelHigh, 0.75: LevelHigh, 0.6: LevelLikely,
		0.5: LevelLikely, 0.46: LevelPossible, 0: LevelNone,
	}
	for confidence, want := range cases {
		assert.Equal(t, want, ConfidenceLevel(confidence), confidence)
	}
}

func TestScoreCasesCountsCorrectAndWrongHigh(t *testing.T) {
	accuracy := ScoreCases([]Case{
		{Name: "a", Correct: true, Confidence: 0.9},
		{Name: "b", Correct: false, Confidence: 0.8},
		{Name: "c", Correct: false, Confidence: 0.6},
		{Name: "d", Correct: true},
	})
	assert.Equal(t, 4, accuracy.Cases)
	assert.Equal(t, 2, accuracy.Correct)
	assert.Equal(t, 1, accuracy.WrongHigh)
	assert.InDelta(t, 50, accuracy.CorrectPercent(), 0.001)
	assert.Equal(t, 2, accuracy.HighCases())
	assert.InDelta(t, 50, accuracy.WrongHighPercent(), 0.001,
		"one of the two high-confidence cases is wrong")
}

func TestScoreCasesCalibrationPerLevel(t *testing.T) {
	accuracy := ScoreCases([]Case{
		{Correct: true, Confidence: 0.9},
		{Correct: false, Confidence: 0.9},
		{Correct: true, Confidence: 0.55},
	})
	levels := map[string]Level{}
	for _, level := range accuracy.Levels {
		levels[level.Name] = level
	}
	assert.False(t, levels[LevelHigh].Calibrated(),
		"50% right at high confidence is overconfident")
	assert.True(t, levels[LevelLikely].Calibrated())
	assert.True(t, levels[LevelNone].Calibrated(), "no cases, no promise")
	assert.False(t, accuracy.Calibrated())
}

func TestScoreCasesWithoutCases(t *testing.T) {
	accuracy := ScoreCases(nil)
	assert.Zero(t, accuracy.CorrectPercent())
	assert.True(t, accuracy.Calibrated())
}

// A score that is 0.7 up to rounding is high, as in the calibration
// buckets; the same score cannot be "likely" in one and 0.70 in another.
func TestConfidenceLevelToleratesRounding(t *testing.T) {
	assert.Equal(t, LevelHigh, ConfidenceLevel(0.6999999999999))
	assert.Equal(t, LevelLikely, ConfidenceLevel(0.4999999999999))
	assert.Equal(t, LevelPossible, ConfidenceLevel(0.49))
	assert.Equal(t, LevelNone, ConfidenceLevel(0))
}

// The high level promises what CalibratedHigh demands (80%), not its own
// 70% floor: 3 of 4 right (75%) at high confidence is not calibrated.
func TestHighLevelPromisesTheCalibrationBar(t *testing.T) {
	accuracy := ScoreCases([]Case{
		{Correct: true, Confidence: 0.8}, {Correct: true, Confidence: 0.8},
		{Correct: true, Confidence: 0.8}, {Correct: false, Confidence: 0.8},
	})
	assert.False(t, accuracy.Calibrated())

	accuracy = ScoreCases([]Case{
		{Correct: true, Confidence: 0.8}, {Correct: true, Confidence: 0.8},
		{Correct: true, Confidence: 0.8}, {Correct: true, Confidence: 0.8},
		{Correct: false, Confidence: 0.8},
	})
	assert.True(t, accuracy.Calibrated(), "exactly 80%")
}
