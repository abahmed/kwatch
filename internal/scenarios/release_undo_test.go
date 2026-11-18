package scenarios

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// undoOffered reports a step that undoes a release: a rollout undo, a
// Helm rollback or a revert in Git.
func undoOffered(steps []notification.Step) bool {
	for _, s := range steps {
		if strings.Contains(s.Command, "rollout undo") ||
			strings.HasPrefix(s.Command, "helm rollback") ||
			strings.HasPrefix(s.Text, "Revert the change in Git") {
			return true
		}
	}
	return false
}

// assertReadOnlyRelease checks every message of a release kwatch must
// not offer to undo: no undo in the steps a provider gets nor in the
// steps the dashboard stores, the read-only rollout check first.
func assertReadOnlyRelease(t *testing.T, result replay.Result) {
	t.Helper()
	require.NotEmpty(t, result.Messages)
	for _, m := range result.Messages {
		assert.False(t, undoOffered(m.Steps), "undo in %+v", m.Steps)
		for _, s := range pipeline.FixSteps(m) {
			assert.NotContains(t, s.Command, "rollout undo")
			assert.NotContains(t, s.Command, "helm rollback")
		}
		assert.NotContains(t, m.Note, "Rolling back")
		require.NotEmpty(t, m.Steps)
		assert.Equal(t, "kubectl rollout status deployment/checkout "+
			"-n shop", m.Steps[0].Command)
	}
}

// A release blamed only at "possible" gets the read-only checks and
// what would confirm it, never a rollback.
func TestPossibleReleaseScenarioOffersNoRollback(t *testing.T) {
	result := replayNamed(t, "release-blamed-possible")
	assertReadOnlyRelease(t, result)
	cause := result.Decisions[0].Incident.Cause
	require.NotNil(t, cause)
	require.NotNil(t, cause.Change)
	assert.Equal(t, rootcause.LevelPossible, rootcause.CertaintyOf(cause))
	assert.Equal(t, "13", cause.RollbackRevision)
	assert.True(t, cause.PreviousServed)
	var confirm []string
	for _, s := range result.Messages[0].Steps {
		if strings.Contains(s.Text, "confirms the release is the cause") {
			confirm = append(confirm, s.Command)
		}
	}
	assert.Equal(t, []string{"kubectl rollout history deployment/checkout " +
		"-n shop --revision=13"}, confirm)
}

// A release whose previous revision kwatch never saw run gets no
// rollback; the message says why in plain words.
func TestNeverReadyPreviousRevisionScenarioOffersNoRollback(t *testing.T) {
	result := replayNamed(t, "release-previous-never-ready")
	assertReadOnlyRelease(t, result)
	cause := result.Decisions[0].Incident.Cause
	require.NotNil(t, cause)
	assert.Equal(t, "13", cause.RollbackRevision)
	assert.False(t, cause.PreviousServed)
	assert.NotEqual(t, rootcause.LevelPossible, rootcause.CertaintyOf(cause))
	assert.Contains(t, result.Messages[0].Note, "kwatch has no record of "+
		"revision 13 running healthily, so it does not suggest rolling "+
		"back to it.")
}

// The opposite cases keep their rollback: a likely or high cause whose
// previous revision served.
func TestServedPreviousRevisionKeepsTheRollback(t *testing.T) {
	for name, want := range map[string]string{
		"release-regression": "kubectl rollout undo deployment/checkout " +
			"-n shop --to-revision=13",
		"bad-config-value-change": "kubectl rollout undo " +
			"deployment/checkout -n shop --to-revision=13",
		"release-helm-managed": "helm rollback billing 1 -n shop",
	} {
		t.Run(name, func(t *testing.T) {
			result := replayNamed(t, name)
			require.NotEmpty(t, result.Messages)
			var commands []string
			for _, s := range result.Messages[0].Steps {
				commands = append(commands, s.Command)
			}
			assert.Contains(t, commands, want)
		})
	}
}
