package slack

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// Slack rejects the whole message with invalid_blocks when any single limit is
// exceeded — the alert is lost, not degraded. In production this dropped 40 of
// 198 notifications in one day. These tests pin every limit.

func payloadStats(
	b *slackClient.Blocks,
) (maxFields, maxFieldChars, maxSectionChars, blocks int) {
	blocks = len(b.BlockSet)
	for _, blk := range b.BlockSet {
		sec, ok := blk.(slackClient.SectionBlock)
		if !ok {
			continue
		}
		if len(sec.Fields) > maxFields {
			maxFields = len(sec.Fields)
		}
		for _, f := range sec.Fields {
			if n := utf8.RuneCountInString(f.Text); n > maxFieldChars {
				maxFieldChars = n
			}
		}
		if sec.Text != nil {
			if n := utf8.RuneCountInString(sec.Text.Text); n > maxSectionChars {
				maxSectionChars = n
			}
		}
	}
	return
}

func hostileIncident(lines int, lineLen int) notification.Message {
	m := notification.Message{
		Key: "incident-1", Status: notification.StatusCritical,
		Title:      strings.Repeat("api is failing ", lineLen/15),
		Confidence: "high",
	}
	for i := 0; i < lines; i++ {
		m.Lines = append(m.Lines, strings.Repeat("x", lineLen))
		m.Timeline = append(m.Timeline, fmt.Sprintf(
			"23:%02d FailedScheduling 0/7 nodes are available", i%60))
		m.Output = append(m.Output, strings.Repeat("log ", lineLen/4))
		m.Steps = append(m.Steps, notification.Step{
			Text: "check the node", Command: "kubectl get nodes",
		})
	}
	return m
}

func TestSlackPayloadStaysWithinEveryLimit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lines   int
		lineLen int
	}{
		{"typical", 5, 60},
		{"busy incident", 400, 60},
		{"long lines", 20, 5000},
		{"pathological", 2000, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := hostileIncident(tc.lines, tc.lineLen)
			for _, b := range []*slackClient.Blocks{
				rootBlocks(m), noteBlocks(m),
			} {
				fields, fieldChars, sectionChars, blocks := payloadStats(b)
				assert.LessOrEqual(t, fields, maxFieldsPerSection)
				assert.LessOrEqual(t, fieldChars, maxFieldChars)
				assert.LessOrEqual(t, sectionChars, maxSectionTextChars)
				assert.LessOrEqual(t, blocks, maxBlocksPerMessage)
			}
		})
	}
}

func TestTruncateMrkdwnIsRuneSafe(t *testing.T) {
	// Multi-byte characters must never be split; that produces invalid UTF-8
	// which Slack also rejects.
	s := strings.Repeat("é", maxFieldChars+50)
	out := truncateMrkdwn(s, maxFieldChars)
	require.True(t, utf8.ValidString(out))
	assert.LessOrEqual(t, utf8.RuneCountInString(out), maxFieldChars)
	assert.True(t, strings.HasSuffix(out, "..."))
	assert.Equal(t, "short", truncateMrkdwn("short", maxFieldChars))
}

func TestTruncateMrkdwnNeverSplitsAnEntity(t *testing.T) {
	escaped := escapeMrkdwn(strings.Repeat("a<", 20))
	for limit := 4; limit < 40; limit++ {
		out := strings.TrimSuffix(truncateMrkdwn(escaped, limit), "...")
		if amp := strings.LastIndexByte(out, '&'); amp >= 0 {
			require.Contains(t, out[amp:], ";",
				"limit %d cut an entity: %q", limit, out)
		}
		require.LessOrEqual(t, utf8.RuneCountInString(out)+3, limit)
	}
}

func TestSlackSectionsEscapeBeforeTruncating(t *testing.T) {
	// Every "&" grows to five characters when escaped. Cutting first and
	// escaping after would send far more than the section limit.
	m := providertest.Announce()
	m.Note = strings.Repeat("&", maxSectionTextChars)
	m.Output = []string{strings.Repeat("<", maxSectionTextChars)}
	blocks := noteBlocks(m).BlockSet
	require.Len(t, blocks, 3, "note, output label, output")
	for _, block := range blocks {
		text := block.(slackClient.SectionBlock).Text.Text
		assert.LessOrEqual(t,
			utf8.RuneCountInString(text), maxSectionTextChars)
	}
	code := blocks[2].(slackClient.SectionBlock).Text.Text
	assert.True(t, strings.HasPrefix(code, "```"))
	assert.True(t, strings.HasSuffix(code, "...```"),
		"the closing fence must survive truncation")
}

func TestCapBlocksAnnouncesWhatItDropped(t *testing.T) {
	blocks := make([]slackClient.Block, 0, maxBlocksPerMessage+20)
	for i := 0; i < maxBlocksPerMessage+20; i++ {
		blocks = append(blocks, markdownSection(fmt.Sprintf("block %d", i)))
	}
	capped := capBlocks(blocks)
	require.Len(t, capped, maxBlocksPerMessage)
	last := capped[len(capped)-1].(slackClient.SectionBlock)
	assert.Contains(
		t,
		last.Text.Text,
		"21 more block(s) omitted",
		"a trimmed alert must say so",
	)
}
