package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/rootcause"
)

// A revision kwatch saw serving keeps backing a rollback after a
// completed rollout scaled it to zero and removed its pods: the bad
// release after it is a likely cause and the undo goes to it.
func TestCompletedRolloutKeepsTheRollbackEvidence(t *testing.T) {
	result := replayNamed(t, "release-after-completed-rollout")
	require.NotEmpty(t, result.Decisions)
	cause := result.Decisions[0].Incident.Cause
	require.NotNil(t, cause)
	require.NotNil(t, cause.Change)
	assert.Equal(t, "14", cause.RollbackRevision)
	assert.True(t, cause.PreviousServed)
	assert.Contains(t, []rootcause.Level{rootcause.LevelLikely,
		rootcause.LevelHigh}, rootcause.CertaintyOf(cause))
	var commands []string
	for _, s := range result.Messages[0].Steps {
		commands = append(commands, s.Command)
	}
	assert.Contains(t, commands, "kubectl rollout undo deployment/checkout "+
		"-n shop --to-revision=14")
}

// A StatefulSet release whose previous revision crashes the same way is
// not blamed and gets no rollback; the message says both versions fail.
func TestStatefulSetPreviousRevisionFailsDropsTheRelease(t *testing.T) {
	result := replayNamed(t, "statefulset-release-previous-fails")
	require.NotEmpty(t, result.Decisions)
	cause := result.Decisions[0].Incident.Cause
	require.NotNil(t, cause)
	assert.Nil(t, cause.Change)
	assert.Empty(t, cause.RollbackRevision)
	assert.True(t, cause.PreviousFails)
	for _, m := range result.Messages {
		assert.False(t, undoOffered(m.Steps), "undo in %+v", m.Steps)
	}
	assert.Contains(t, result.Messages[0].Note,
		"The previous version fails the same way")
}
