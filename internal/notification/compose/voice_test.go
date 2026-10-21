package compose

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// firstPerson matches the writer speaking as "I".
var firstPerson = regexp.MustCompile(`\bI('ll|'m|'ve|'d| can| cannot| ` +
	`could| will| am| have| did)\b`)

// A note speaks of kwatch in the third person, never as "I".
func TestNotesNeverSpeakInTheFirstPerson(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			note := c.write().Note

			assert.False(t, firstPerson.MatchString(note), note)
		})
	}
}

func TestFlappingUpdatePromisesInTheThirdPerson(t *testing.T) {
	note := writeFlappingUpdate().Note

	assert.Contains(t, note, "kwatch will write again when it stays "+
		"healthy or fails differently.")
}
