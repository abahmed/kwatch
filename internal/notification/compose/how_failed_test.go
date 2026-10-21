package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
)

func TestHowFailedSaysRunLengthAndRhythm(t *testing.T) {
	got := howFailedText(detection.Finding{Evidence: []detection.Evidence{
		{Label: detection.EvidenceRanFor, Value: "4s"},
		{Label: detection.EvidenceRestartSpan, Value: "2h10m"},
		{Label: "restarts", Value: "14"},
	}}, "")

	assert.Equal(t, "Its last run lasted four seconds; 14 restarts "+
		"since its pod was created two hours ago.", got)
}

func TestHowFailedSaysNothingWithoutFacts(t *testing.T) {
	assert.Empty(t, howFailedText(detection.Finding{}, ""))
}

func TestMemoryJumpIsToldBeforeASteadyRise(t *testing.T) {
	got := memoryText(detection.Finding{Evidence: []detection.Evidence{
		{Label: detection.EvidenceMemoryJump,
			Value: "180Mi to 512Mi within 40s"},
		{Label: detection.EvidenceMemoryRise,
			Value: "200Mi to 512Mi over 3h"},
	}})

	assert.Equal(t, "Its memory jumped from 180Mi to 512Mi within 40s "+
		"before the kill.", got)
}
