package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// The rollback offer says what the command does, never that it fixes.
func TestRollbackOfferSaysWhatTheCommandDoes(t *testing.T) {
	msg := Writer{}.Write(announce(badRollout()), at(5, 0))

	assert.Contains(t, msg.Note, "Rolling back to revision 13, the one "+
		"before this release, undoes it (changes the cluster): "+
		"kubectl rollout undo deployment/payments -n shop "+
		"--to-revision=13")
	assert.NotContains(t, msg.Note, "fixes it")
	assert.NotContains(t, msg.Note, "should fix")
}

func TestRollbackOfferOfALikelyCauseSaysSo(t *testing.T) {
	p := badRollout()
	p.Cause.Score = 0.55
	p.Tier = incident.Notify

	note := Writer{}.Write(announce(p), at(5, 0)).Note

	assert.Contains(t, note, "undoes it; the release is the likely cause "+
		"(changes the cluster): kubectl rollout undo")
}
