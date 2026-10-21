package scorecard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCaseWrongStatedNeedsALikelyOrHigherWrongRoot(t *testing.T) {
	cases := map[string]struct {
		c    Case
		want bool
	}{
		"right at high":        {Case{Correct: true, Confidence: 0.9}, false},
		"wrong at high":        {Case{Confidence: 0.9}, true},
		"wrong at likely":      {Case{Confidence: 0.55}, true},
		"wrong at possible":    {Case{Confidence: 0.45}, false},
		"wrong with no cause":  {Case{}, false},
		"hedge, not ambiguous": {Case{Confidence: 0.69, Hedged: true}, true},
		"ambiguous, no hedge":  {Case{Confidence: 0.69, Ambiguous: true}, true},
		"hedge, ambiguous": {Case{Confidence: 0.69, Hedged: true,
			Ambiguous: true}, false},
		"wrong at high, hedged": {Case{Confidence: 0.8, Hedged: true,
			Ambiguous: true}, false},
	}
	for name, c := range cases {
		assert.Equal(t, c.want, c.c.WrongStated(), name)
	}
}

func TestScoreCasesSeparatesFoundFromFallbackAndUnknown(t *testing.T) {
	accuracy := ScoreCases([]Case{
		{Correct: true, Confidence: 0.9, ExpectsRoot: true},
		{Correct: true, Confidence: 0.6, ExpectsRoot: true},
		{Correct: true, ExpectsRoot: true},
		{Correct: true, ExpectsRoot: true},
		{Correct: true},
		{Confidence: 0.6, ExpectsRoot: true},
		{Confidence: 0.69, Hedged: true, Ambiguous: true},
		{Confidence: 0.2, ExpectsRoot: true},
	})
	assert.Equal(t, 5, accuracy.Correct)
	assert.Equal(t, 2, accuracy.CorrectStated)
	assert.Equal(t, 2, accuracy.CorrectFallback,
		"no stated cause for a label that names a root is a fallback")
	assert.Equal(t, 1, accuracy.CorrectUnknown)
	assert.Equal(t, 1, accuracy.WrongStated)
	assert.Equal(t, 1, accuracy.ToleratedHedges)
	assert.Zero(t, accuracy.WrongHigh)
}

// The count gate has no slack: one wrong root among thousands of right
// ones still fails it, which a percentage would not.
func TestWrongStatedGateHasNoSlack(t *testing.T) {
	cases := make([]Case, 0, 1001)
	for range 1000 {
		cases = append(cases,
			Case{Correct: true, Confidence: 0.9, ExpectsRoot: true})
	}
	assert.True(t, AtMost("wrong stated",
		float64(ScoreCases(cases).WrongStated),
		GoalWrongStatedRoots, "").Pass)
	cases = append(cases, Case{Confidence: 0.75, ExpectsRoot: true})
	accuracy := ScoreCases(cases)
	assert.False(t, AtMost("wrong stated", float64(accuracy.WrongStated),
		GoalWrongStatedRoots, "").Pass)
	assert.Less(t, accuracy.WrongHighPercent(), float64(GoalWrongHighPercent),
		"the percentage gate would have let it pass")
}
