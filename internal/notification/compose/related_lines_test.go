package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// nodeCause is an open node incident people already heard about.
var nodeCause = incident.Related{ID: "node-1",
	Title: "Node n1 (prod-eu-1) is not ready.", Tier: incident.Page,
	State: incident.Open, Told: true}

func blockTexts(msg notification.Message) []string {
	out := make([]string, 0, len(msg.Doc))
	for _, b := range msg.Doc {
		out = append(out, b.Text())
	}
	return out
}

func TestWriterSaysWhatCausedAnAnnouncement(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	d.Incident.CausedBy = []incident.Related{nodeCause}

	msg := linkWriter(linkBase).Write(d, d.Incident.Opened)

	assert.Contains(t, msg.Note, "Likely caused by: Node n1 "+
		"(prod-eu-1) is not ready; kwatch is still checking.")
	assert.Contains(t, blockTexts(msg),
		"Cause in dashboard: "+linkBase+"/issues/node-1")
	assert.Equal(t, "Open in dashboard: "+linkBase+"/issues/rb",
		lastBlockText(msg), "the incident's own link stays last")
}

func TestWriterNamesNoCausePeopleNeverHeardOf(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	unheard := nodeCause
	unheard.Told = false
	d.Incident.CausedBy = []incident.Related{unheard}

	msg := linkWriter(linkBase).Write(d, d.Incident.Opened)

	assert.NotContains(t, msg.Note, "Caused by")
	assert.False(t, slices.ContainsFunc(blockTexts(msg), func(s string) bool {
		return s == "Cause in dashboard: "+linkBase+"/issues/node-1"
	}))
}

func TestWriterKeepsCauseOutOfOtherUpdates(t *testing.T) {
	d := runbookDecision(incident.Update, "OOMKilled")
	d.Reason = incident.ReasonReminder
	d.Incident.CausedBy = []incident.Related{nodeCause}

	msg := linkWriter(linkBase).Write(d, d.Incident.Opened)

	assert.NotContains(t, msg.Note, "Caused by")
}

// heardTimes gives d's incident n earlier announced occurrences this
// week.
func heardTimes(d *incident.Decision, n int) {
	for i := range n {
		opened := d.Incident.Opened.Add(-time.Duration(i+1) * 24 *
			time.Hour)
		d.Incident.History = append(d.Incident.History,
			incident.Occurrence{Opened: opened, Resolved: opened.Add(
				10 * time.Minute), Heard: true})
	}
}

func TestWriterNamesARepeatOffender(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	heardTimes(&d, 3)

	msg := linkWriter(linkBase).Write(d, d.Incident.Opened)

	assert.Contains(t, msg.Note,
		"This is the fourth time this week on pod/api-1.")
	assert.Contains(t, blockTexts(msg), "Earlier times: "+linkBase+
		"/issues?status=all&like=rb")
}

func TestWriterLinksNoEarlierTimesBelowTheRepeatCount(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	heardTimes(&d, 2)

	msg := linkWriter(linkBase).Write(d, d.Incident.Opened)

	assert.Contains(t, msg.Note, "This is the third time this week.")
	for _, text := range blockTexts(msg) {
		assert.NotContains(t, text, "Earlier times")
	}
}

func TestWriterCountsTheWeekWhenFailingAgain(t *testing.T) {
	d := runbookDecision(incident.Update, "OOMKilled")
	d.Reason = incident.ReasonFailingAgain
	d.Incident.RepeatCount = 2
	d.Incident.CausedBy = []incident.Related{nodeCause}
	heardTimes(&d, 4)

	msg := Writer{}.Write(d, d.Incident.Opened)

	assert.Contains(t, msg.Note,
		"This is the fifth time this week on pod/api-1.")
	assert.Contains(t, msg.Note, "Likely caused by: Node n1")
}

func TestWriterSaysHowManyPodsAResolveAffected(t *testing.T) {
	d := runbookDecision(incident.Resolve, "OOMKilled")
	d.Incident.Opened = d.Incident.Resolved.Add(-10 * time.Minute)
	d.Incident.Journal.Widest = incident.Radius{Pods: 3}

	msg := Writer{}.Write(d, d.Incident.Resolved)
	assert.Contains(t, msg.Note, "Three pods were affected.")

	d.Incident.Journal.Widest = incident.Radius{Pods: 1}
	msg = Writer{}.Write(d, d.Incident.Resolved)
	assert.NotContains(t, msg.Note, "affected", "one pod is the subject")
}
