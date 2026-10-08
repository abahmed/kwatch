package announce

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

var foldNow = time.Date(2026, 10, 8, 8, 31, 0, 0, time.UTC)

// foldHarness is a collector over a manager that restored records, with a
// sink that keeps every message that is not only an audit record.
func foldHarness(
	records ...incident.Record,
) (*Collector, *[]notification.Message) {
	m := incident.NewManager(incident.Config{}, nil)
	m.Restore(records, foldNow)
	var sent []notification.Message
	writer := compose.Writer{}
	c := New(Env{
		Incidents: m, Messages: writer,
		Write: writer.Write, SaveStartup: func(StartupState) {},
		Sink: func(
			_ context.Context, _ incident.Decision, msg notification.Message,
		) {
			if msg.Carrier == "" {
				sent = append(sent, msg)
			}
		},
	})
	return c, &sent
}

func demotedRecord(id string) incident.Record {
	return incident.Record{ID: id, Tier: incident.Digest,
		State: incident.Open, Opened: foldNow.Add(-48 * time.Hour),
		Announced: foldNow.Add(-48 * time.Hour), Demoted: true,
		Root: inventory.CoreID(kube.KindDeployment, "shop", id)}
}

func causeRevised(c *Collector, id string) incident.Decision {
	return incident.Decision{Action: incident.Update,
		Reason: incident.ReasonCauseRevised, Thread: true,
		Incident: incident.Incident{ID: id, Tier: incident.Digest,
			Root: inventory.CoreID(kube.KindDeployment, "shop", id)}}
}

// After a restart a restored incident that falls to the digest tier posts
// nothing of its own: its thread may never have been told, and the next
// digest lists it as still failing.
func TestRestoredDemotionPostsNothingIndividually(t *testing.T) {
	c, _ := foldHarness(demotedRecord("a"), demotedRecord("b"))

	rest, _ := c.CollectDigest(context.Background(), foldNow,
		[]incident.Decision{causeRevised(c, "a"), causeRevised(c, "b")})

	assert.Empty(t, rest, "no per-incident post for restored demotions")
}

// Only the first demotion is folded: a later cause revision of the same
// incident is real thread news again.
func TestRestoredDemotionFoldsOnlyOnce(t *testing.T) {
	c, _ := foldHarness(demotedRecord("a"))
	ctx := context.Background()

	c.CollectDigest(ctx, foldNow, []incident.Decision{causeRevised(c, "a")})
	rest, _ := c.CollectDigest(ctx, foldNow.Add(time.Hour),
		[]incident.Decision{causeRevised(c, "a")})

	assert.Len(t, rest, 1)
}

// An incident that did not come from the state file keeps its thread
// news: it was demoted by this run, in its own thread.
func TestLiveDemotionStillTellsItsThread(t *testing.T) {
	c, _ := foldHarness()

	rest, _ := c.CollectDigest(context.Background(), foldNow,
		[]incident.Decision{causeRevised(c, "live")})

	assert.Len(t, rest, 1)
}

func oldFinding(since time.Time) incident.Decision {
	root := inventory.CoreID(kube.KindSecret, "arc-system", "test-secret")
	finding := detection.Finding{Entity: root, Reason: "StuckDeleting",
		Since: since}
	return incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{ID: "inc-7743", Root: root,
			Tier: incident.Notify, Opened: foldNow.Add(-time.Minute),
			Members: map[detection.Key]detection.Finding{
				finding.Key(): finding}}}
}

// A problem found after a restart that has been there for months is not
// news of its own: the restored-incidents message names it, as it names
// its siblings.
func TestOldProblemFoundAfterRestartJoinsTheRestoredSummary(t *testing.T) {
	c, sent := foldHarness()
	c.Startup.WarmAt = foldNow.Add(10 * time.Minute)
	ctx := context.Background()
	old := oldFinding(foldNow.Add(-300 * 24 * time.Hour))

	rest, _ := c.CollectStartup(ctx, foldNow, []incident.Decision{old})

	assert.Empty(t, rest)
	assert.Empty(t, *sent, "nothing is posted at once")
	require.True(t, c.ListRestored(ctx, c.Startup.WarmAt))
	require.Len(t, *sent, 1)
	assert.Contains(t, (*sent)[0].Note, "test-secret")
}

// A problem that began after kwatch started is news and goes out at once.
func TestNewProblemAfterRestartIsAnnounced(t *testing.T) {
	c, sent := foldHarness()
	c.Startup.WarmAt = foldNow.Add(10 * time.Minute)

	rest, _ := c.CollectStartup(context.Background(), foldNow,
		[]incident.Decision{oldFinding(foldNow)})

	assert.Len(t, rest, 1)
	assert.Empty(t, *sent)
}

// "opened" counts what opened in the window; a problem told before and
// still failing is ongoing.
func TestDigestCountsOnlyIncidentsOpenedInTheWindow(t *testing.T) {
	c, sent := foldHarness()
	now := foldNow
	fresh := causeRevised(c, "fresh")
	fresh.Action, fresh.Reason = incident.Announce, incident.ReasonSettled
	old := causeRevised(c, "old")
	old.Reason = incident.ReasonReminder
	c.Low.Opened = []incident.Decision{fresh, old}
	c.Low.Since = now.Add(-DigestWindow)

	require.True(t, c.flushDigest(context.Background(), now))

	require.Len(t, *sent, 1)
	listed := (*sent)[0].Listed
	assert.Equal(t, 1, listed.Opened)
	assert.Equal(t, 1, listed.Ongoing)
}

// A restored incident keeps the time it opened, and the ongoing line says
// it in the incident's own words: no reason code, the original start.
func TestOngoingLineKeepsTheOriginalStartAndSaysNoReasonCode(t *testing.T) {
	record := demotedRecord("web")
	record.DigestedAt = foldNow.Add(-time.Hour)
	c, _ := foldHarness(record)

	got := c.ongoingProblems(foldNow)

	require.Len(t, got, 1)
	assert.Equal(t, record.Opened, got[0].Since)
	assert.Contains(t, got[0].Line(foldNow), "still failing, for two days")
}
