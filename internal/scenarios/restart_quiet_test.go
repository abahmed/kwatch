package scenarios

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/replay"
)

// restartAfterTold runs a first session that told people about the
// loud invoicer problem (and, with digest, about a lower-priority
// autoscaler), then restarts kwatch 90 s later. news adds what kwatch
// finds on its first list after the restart, such as a problem that
// began while it was down.
func restartAfterTold(
	t *testing.T, digest bool, news func(again *cluster),
) replay.Result {
	t.Helper()
	store := newPersistingStore()
	_, failing := restartFirstSession()
	c := failing.cluster
	if digest {
		c.list(agedMissingTargetHPA(c))
	}
	c.after(45 * time.Minute)
	first := c.log()
	firstRun := restartReplay(t, first, store, replay.Options{
		SyncAt: first.Start, Tail: 5 * time.Minute})

	restartAt := firstRun.End.Add(restartGap)
	again := newCluster(scenarioStart, "")
	again.now = restartAt
	again.list(remaining(c)...)
	if news != nil {
		news(again)
	}
	again.after(30 * time.Minute)
	second := again.log()
	second.Start = restartAt.UTC()
	return restartReplay(t, second, store, replay.Options{
		SyncAt: restartAt, Tail: 20 * time.Minute})
}

// restartMessages are the messages a restart sent, by title.
func restartMessages(run replay.Result) []string {
	var out []string
	for _, m := range run.Messages {
		if strings.Contains(m.Title, "restarted") {
			out = append(out, m.Title+" "+m.Note)
		}
	}
	return out
}

// quietRestarts counts the audit-only quiet-restart entries.
func quietRestarts(run replay.Result) int {
	n := 0
	for _, c := range run.Carried {
		if c.Carrier == announce.CarrierQuietRestart {
			n++
		}
	}
	return n
}

// A short restart over problems people already heard of, one loud and
// one lower-priority, with nothing new or worse says nothing at all:
// no message, only the audit log's quiet-restart entry.
func TestRestartWithNothingNewSendsNoMessage(t *testing.T) {
	run := restartAfterTold(t, true, nil)

	assert.Empty(t, run.Messages, "a quiet restart sent a message: %s",
		describeDecisions(run))
	assert.Equal(t, 1, quietRestarts(run), "the audit log has no entry")
}

// A loud problem that began while kwatch was down is told once, at its
// own tier, as a message of its own after the restore grace: the one
// message that day. The restart adds nothing to it, so the problems
// that were told and are unchanged (the loud one and the
// lower-priority one) are not repeated in any message, and the audit
// log records the restart as quiet. (When the restart itself has news
// or a downtime gap to report, its one message counts the unchanged
// ones; see announce.TestRestartNamesOnlyNewsAtItsOwnTier.)
func TestRestartTellsOnlyTheNewProblemAndNotTheUnchanged(t *testing.T) {
	run := restartAfterTold(t, true, newLoudProblemWhileDown)

	require.Len(t, run.Messages, 1, "messages: %s", describeDecisions(run))
	assert.Equal(t, "n3", run.Decisions[0].Incident.Root.Name)
	assert.Empty(t, restartMessages(run))
	for _, m := range run.Messages {
		text := m.Title + " " + m.Note
		assert.NotContains(t, text, "invoicer")
		assert.NotContains(t, text, "thirdparty")
	}
	assert.Equal(t, 1, quietRestarts(run))
}

// newLoudProblemWhileDown lists a node that went NotReady hours ago,
// before kwatch came back.
func newLoudProblemWhileDown(c *cluster) {
	node := c.node("n3", "zone-a")
	setNodeCondition(node, corev1.NodeReady, corev1.ConditionFalse,
		"KubeletNotReady", "container runtime is down",
		c.now.Add(-4*time.Hour))
	c.list(node)
}
