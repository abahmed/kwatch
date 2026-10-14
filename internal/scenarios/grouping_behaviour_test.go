package scenarios

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

// pageTexts lists the text of every message, for failure output.
func pageTexts(result replay.Result) string {
	var b strings.Builder
	for i, m := range result.Messages {
		b.WriteString(result.Times[i].Format("15:04:05"))
		b.WriteString(" ")
		b.WriteString(notification.Text(m))
		b.WriteString("\n")
	}
	return b.String()
}

// A flap is paged once and a lasting second outage is paged again: the
// hold on a repeated page ends when the failure outlasts the time a
// recovery needs to be believed.
func TestNodeBlipThenDownPagesAgainWhenItStays(t *testing.T) {
	result := replayNamed(t, "node-blip-then-down")
	assert.Equal(t, 2, pagesRaised(result), pageTexts(result))
}

// Several nodes lost in one minute are one outage and one page.
func TestNodesLostTogetherPageOnce(t *testing.T) {
	for _, name := range []string{
		"nodes-lost-same-zone", "nodes-lost-three-zones",
	} {
		t.Run(name, func(t *testing.T) {
			result := replayNamed(t, name)
			assert.Equal(t, 1, pagesRaised(result), pageTexts(result))
			assert.Zero(t, falseRecoveries(result), pageTexts(result))
		})
	}
}

// Pods evicted by node pressure that stay listed after the pressure
// cleared are history of that incident, not new incidents.
func TestEvictedPodsAfterPressureAreNotNewProblems(t *testing.T) {
	result := replayNamed(t, "node-pressure-evicted-linger")
	announced := decisionsOf(result, incident.Announce, "")
	assert.Len(t, announced, 1, pageTexts(result))
	assert.Equal(t, 2, len(result.Messages), pageTexts(result))
}

// Workloads that crashed before their node was lost join its incident:
// one page, no second story beside it, and its recovery says so.
func TestLateNodeCauseJoinsTheCrashingWorkloads(t *testing.T) {
	result := replayNamed(t, "node-late-cause-after-crashes")
	assert.Equal(t, 1, pagesRaised(result), pageTexts(result))
	assert.Len(t, decisionsOf(result, incident.Announce, ""), 1,
		pageTexts(result))
	for _, d := range decisionsOf(result, incident.Resolve, "") {
		assert.Empty(t, d.Incident.SupersededBy, pageTexts(result))
	}
}
