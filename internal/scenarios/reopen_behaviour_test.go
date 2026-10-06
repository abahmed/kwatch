package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

// replayNamed replays a committed labelled scenario.
func replayNamed(t *testing.T, name string) replay.Result {
	t.Helper()
	log, e := loadScenario(t, name)
	return replayLog(t, log, e.options(log.Start))
}

// decisionsOf lists the decisions with action and reason ("" is any).
func decisionsOf(
	result replay.Result, action incident.Action,
	reason incident.Reason,
) []incident.Decision {
	var out []incident.Decision
	for _, d := range result.Decisions {
		if d.Action == action && (reason == "" || d.Reason == reason) {
			out = append(out, d)
		}
	}
	return out
}

// A page that resolves and fails again pages once. Every later message
// belongs to the same incident: one thread, one alert. The first return
// is told; the second comes back inside the longer hold of an incident
// that already re-opened, so it stays in the open incident and adds no
// message of its own.
func TestPageFlapsReopensTheSameIncident(t *testing.T) {
	assertSamePageReopens(t, replayNamed(t, "page-flaps-reopens"), 1)
}

// Three returns, generated in memory and not part of the scorecard,
// behave the same: the returns inside a hold are not told again.
func TestPageFlapsThreeTimesStillPagesOnce(t *testing.T) {
	c := newCluster(scenarioStart, "")
	buildPageFlaps(c, 3)
	result := replayLog(t, c.log(), replay.Options{Tail: 10 * time.Minute})
	assertSamePageReopens(t, result, 2)
}

// assertSamePageReopens checks a replay of a page that returned: one
// announcement and page, times "failing again" updates, all under one
// incident, key and alert key.
func assertSamePageReopens(
	t *testing.T, result replay.Result, times int,
) {
	t.Helper()
	announced := decisionsOf(result, incident.Announce, "")
	require.Len(t, announced, 1, "one announcement, so one page")
	assert.Equal(t, incident.Page, announced[0].Incident.Tier)
	id := announced[0].Incident.ID

	again := decisionsOf(result, incident.Update, incident.ReasonFailingAgain)
	assert.Len(t, again, times, "each return is an update")
	for _, d := range again {
		assert.Equal(t, incident.Notify, d.Incident.Tier, "no extra page")
	}
	for i, d := range result.Decisions {
		assert.Equal(t, id, d.Incident.ID, "decision %d", i)
		assert.Equal(t, result.Messages[0].Key, result.Messages[i].Key)
		assert.Equal(t, result.Messages[0].DedupKey,
			result.Messages[i].DedupKey, "one paging alert key")
	}
	assert.Len(t, result.Incidents, 1)
}

// A page that stays open for seven hours is said once more at six hours,
// and nothing else is sent.
func TestLongPageGetsOneReminderAtSixHours(t *testing.T) {
	result := replayNamed(t, "long-page-reminder")

	require.Len(t, result.Messages, 2)
	assert.Equal(t, incident.Announce, result.Decisions[0].Action)
	assert.Equal(t, incident.Update, result.Decisions[1].Action)
	assert.Equal(t, incident.ReasonReminder, result.Decisions[1].Reason)
	wait := result.Times[1].Sub(result.Times[0])
	assert.Equal(t, incident.PageRemindAfter, wait)
	assert.GreaterOrEqual(t, result.End.Sub(result.Times[0]), 7*time.Hour,
		"the page stayed open for the seven hours")
}

// A digest-tier incident never reaches a paging provider, so its resolve
// must not either: the digest carries it, and any resolve message of its
// own is marked to skip paging.
func TestDigestIncidentResolveSkipsPaging(t *testing.T) {
	result := replayNamed(t, "digest-never-paged-resolve")

	require.Len(t, result.Incidents, 1)
	assert.False(t, result.Incidents[0].Delivery.OpenAtPagers(),
		"no announcement of it reached a paging provider")
	for i, d := range result.Decisions {
		if d.Action == incident.Resolve {
			assert.True(t, result.Messages[i].SkipPaging,
				"resolve %d would close an alert nobody opened", i)
		}
	}
	var resolved bool
	for _, c := range result.Carried {
		resolved = resolved || (c.Decision.Action == incident.Resolve &&
			c.Carrier == "digest")
	}
	assert.True(t, resolved, "the digest carried the resolve")
}

// An incident a roll-up announced gets its own update when its failing
// pods triple, and not before.
func TestRolledUpIncidentWorsensInItsOwnThread(t *testing.T) {
	result := replayNamed(t, "rollup-chronic-worsens")

	var apiUpdates []incident.Decision
	for _, d := range decisionsOf(result, incident.Update, "") {
		if d.Incident.Root.Name == "api" {
			apiUpdates = append(apiUpdates, d)
		}
	}
	require.Len(t, apiUpdates, 1)
	assert.Equal(t, incident.ReasonMaterialChange, apiUpdates[0].Reason)
	assert.Greater(t, apiUpdates[0].Incident.Revision, 1)
}

// An event that recurs after its finding resolved is reported again.
func TestUnusualEventRecurrenceIsReported(t *testing.T) {
	result := replayNamed(t, "unusual-event-recurs")

	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0].Note, "FailedDeployModel")
	require.Len(t, result.Incidents, 1)
	assert.NotEmpty(t, result.Incidents[0].History,
		"it is the second occurrence of the same failure")
}

// A digest-tier incident that was listed, recovers and fails again
// within two hours is the same incident: it is not opened anew, and the
// next digest says it is failing again, once, instead of naming a new
// problem and an old one that resolved.
func TestDigestFlapReopensTheSameIncident(t *testing.T) {
	result := replayNamed(t, "digest-flap-reopens")

	require.Len(t, result.Incidents, 1)
	assert.Empty(t, result.Incidents[0].Previous, "reopened, not recurred")
	assert.Equal(t, 2, result.Incidents[0].RepeatCount)
	var ids []string
	for _, c := range result.Carried {
		assert.Equal(t, incident.Digest, c.Decision.Incident.Tier)
		ids = append(ids, c.Decision.Incident.ID)
	}
	require.Len(t, ids, 3, "announce, resolve and the return")
	assert.Equal(t, []string{ids[0], ids[0], ids[0]}, ids)

	var digests []notification.Message
	for _, m := range result.Messages {
		if m.Status == notification.StatusLow {
			digests = append(digests, m)
		}
	}
	require.Len(t, digests, 2, "one digest before the recovery, one after")
	assert.Contains(t, digests[1].Note, "is failing again: 2nd time",
		"the return is listed as recurring")
	assert.NotContains(t, digests[1].Note, "resolved",
		"it came back, so the earlier resolve is not news")
}
