package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

const linkBase = "https://kwatch.example.com"

func linkWriter(base string) Writer {
	return Writer{Dashboard: func() string { return base }}
}

func lastBlockText(msg notification.Message) string {
	return msg.Doc[len(msg.Doc)-1].Text()
}

func TestWriterLinksIncidentPageWhenDashboardHasAddress(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	for _, action := range []incident.Action{incident.Announce,
		incident.Update, incident.Resolve} {
		d.Action = action
		msg := linkWriter(linkBase).Write(d, d.Incident.Opened)
		assert.Equal(t, "Open in dashboard: "+linkBase+"/issues/rb",
			lastBlockText(msg), action)
		assert.NotContains(t, msg.Note, linkBase, "the note has no links")
		if msg.Opening != nil {
			assert.NotContains(t, msg.Opening.Render(
				notification.PlainDialect()), linkBase)
		}
	}
}

func TestWriterRendersDashboardLinkInEveryDialect(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	msg := linkWriter(linkBase).Write(d, d.Incident.Opened)
	want := linkBase + "/issues/rb"
	for name, dialect := range map[string]notification.Dialect{
		"plain":    notification.PlainDialect(),
		"slack":    notification.SlackDialect(),
		"markdown": notification.MarkdownDialect(nil, "\n\n"),
		"html":     notification.HTMLDialect(nil, false),
		"jira":     notification.JiraDialect(),
	} {
		text := msg.Render(dialect)
		assert.True(t, strings.HasSuffix(text, want), "%s: %q", name, text)
	}
}

func TestWriterAddsNoLinkWithoutDashboardAddress(t *testing.T) {
	d := runbookDecision(incident.Announce, "OOMKilled")
	plain := Writer{}.Write(d, d.Incident.Opened)
	empty := linkWriter("").Write(d, d.Incident.Opened)
	assert.Equal(t, plain.Doc, empty.Doc)
	assert.NotContains(t, plain.Render(notification.PlainDialect()),
		"dashboard")
}

func TestWriterLinksIssueListFromSummaries(t *testing.T) {
	w := linkWriter(linkBase)
	ds := []incident.Decision{leftoverDecision(0, detection.Warning)}
	want := "Open dashboard: " + linkBase + "/issues"
	for name, msg := range map[string]notification.Message{
		"digest":   w.Digest(ds, nil, nil, extrasNow),
		"rollup":   w.Rollup(ds, extrasNow),
		"startup":  w.StartupSummary(ds, extrasNow),
		"restored": w.RestoredSummary(ds, extrasNow),
		"outage": w.NamespaceOutage(NamespaceOutage{Namespace: "ns0",
			Workloads: 1}, ds, extrasNow),
	} {
		assert.Equal(t, want, lastBlockText(msg), name)
	}
	resolved := w.RollupResolved("rollup-1", 2)
	assert.NotContains(t, resolved.Render(notification.PlainDialect()),
		linkBase)
}

func TestWithLinkKeepsPinnedFixTogether(t *testing.T) {
	msg := notification.Message{Doc: []notification.Block{
		{Kind: notification.Para, Spans: []notification.Span{{Text: "a"}}},
		{Kind: notification.CodeBlock, Pinned: true,
			Spans: []notification.Span{{Text: "kubectl rollout undo"}}},
	}}
	got := withLink(msg, "Open in dashboard: x")
	require.Len(t, got.Doc, 3)
	assert.True(t, got.Doc[2].Pinned)
	assert.Len(t, msg.Doc, 2, "the original blocks are not changed")

	msg.Doc[1].Pinned = false
	assert.False(t, withLink(msg, "x").Doc[2].Pinned)
	assert.Empty(t, withLink(notification.Message{}, "x").Doc)
}
