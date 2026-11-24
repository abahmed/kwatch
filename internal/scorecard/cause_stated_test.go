package scorecard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A cause rootcause.Confirmed does not let a message name is not
// stated: the message says kwatch could not confirm a cause.
func TestUnconfirmedCauseIsNotStated(t *testing.T) {
	assert.True(t, Case{Confidence: 0.4}.Stated())
	assert.False(t, Case{Confidence: 0.4, Unconfirmed: true}.Stated())
	assert.False(t, Case{}.Stated())
}

func TestScoreCasesCountsWrongAndNoCauseStated(t *testing.T) {
	accuracy := ScoreCases([]Case{
		{Correct: true, Confidence: 0.9, ExpectsRoot: true},
		{Correct: true, Confidence: 0.4, ExpectsRoot: true,
			Unconfirmed: true},
		{Correct: true, ExpectsRoot: true},
		{Confidence: 0.4, ExpectsRoot: true},
		{Confidence: 0.4, ExpectsRoot: true, Unconfirmed: true},
		{Confidence: 0.69, Hedged: true, Ambiguous: true},
	})
	assert.Equal(t, 1, accuracy.WrongCauseStated,
		"only the confirmed wrong cause; a tolerated hedge names its rival")
	assert.Equal(t, 3, accuracy.NoCauseStated)
	assert.Equal(t, 1, accuracy.CorrectStated)
	assert.Equal(t, 2, accuracy.CorrectFallback,
		"an unconfirmed right cause is told as no cause")
}
