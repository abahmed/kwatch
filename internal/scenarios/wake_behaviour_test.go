package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

// digestsOf are the digests among the messages of a replay.
func digestsOf(result replay.Result) []notification.Message {
	var out []notification.Message
	for _, m := range result.Messages {
		if strings.HasPrefix(m.Key, notification.DigestKeyPrefix) {
			out = append(out, m)
		}
	}
	return out
}

// The morning scale-up holds every pod's probe failures and says so in
// one line of the next digest: nobody is interrupted, nothing is lost.
func TestWakeUpBlipsAreSummarisedInOneDigestLine(t *testing.T) {
	c := newCluster(scenarioStart, "")
	wakeUp(c, wakeFleet(c), -1)

	result := replayLog(t, c.log(), replay.Options{Tail: time.Hour})

	require.Len(t, result.Messages, 1, "one digest, nothing else")
	digest := digestsOf(result)
	require.Len(t, digest, 1)
	assert.Contains(t, digest[0].Note, "Cluster waking up: 6 workloads "+
		"started between 10:30 and 10:31; 6 had brief startup failures, "+
		"all recovered.")
	assert.Empty(t, result.Decisions[0].Incident.ID,
		"the digest is the only decision: no incident announced")
}

// A wake-up with no failing pod has nothing to report.
func TestWakeUpWithoutFailuresSaysNothing(t *testing.T) {
	c := newCluster(scenarioStart, "")
	fleet := wakeFleet(c)
	for _, w := range fleet {
		created := wakeStart(c, w)
		c.after(15 * time.Second)
		wakeReady(c, w, created)
	}

	result := replayLog(t, c.log(), replay.Options{Tail: time.Hour})

	assert.Empty(t, result.Messages)
}

// A failure that outlasts the wake-up is announced as before, and the
// message says the cluster was waking up.
func TestWakeUpCrashLoopMentionsTheWakeUp(t *testing.T) {
	c := newCluster(scenarioStart, "")
	wakeUp(c, wakeFleet(c), 2)

	result := replayLog(t, c.log(), replay.Options{Tail: time.Hour})

	var announced []string
	for _, m := range result.Messages {
		announced = append(announced, m.Note)
	}
	require.NotEmpty(t, announced)
	assert.Contains(t, announced[0], "keeps crashing")
	assert.Regexp(t, "The cluster (is|was) waking up: 6 workloads "+
		"started between 10:30 and 10:31", strings.Join(announced, "\n"))
}
