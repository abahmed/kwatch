package slack

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notice"
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

func hostileStory(lines int, lineLen int) notice.Message {
	m := notice.Message{
		Key: "problem-1", Status: notice.StatusCritical,
		Title:      strings.Repeat("api is failing ", lineLen/15),
		Confidence: "high",
	}
	for i := 0; i < lines; i++ {
		m.Lines = append(m.Lines, strings.Repeat("x", lineLen))
		m.Timeline = append(m.Timeline, fmt.Sprintf(
			"23:%02d FailedScheduling 0/7 nodes are available", i%60))
		m.Output = append(m.Output, strings.Repeat("log ", lineLen/4))
		m.Steps = append(m.Steps, notice.Step{
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
		{"busy problem", 400, 60},
		{"long lines", 20, 5000},
		{"pathological", 2000, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := hostileStory(tc.lines, tc.lineLen)
			for _, b := range []*slackClient.Blocks{
				storyRootBlocks(m), storyDetailBlocks(m),
			} {
				fields, fieldChars, sectionChars, blocks := payloadStats(b)
				assert.LessOrEqual(t, fields, maxFieldsPerSection)
				assert.LessOrEqual(t, fieldChars, maxFieldChars)
				assert.LessOrEqual(t, sectionChars, 3000)
				assert.LessOrEqual(t, blocks, maxBlocksPerMessage)
			}
		})
	}
}

func TestTruncateFieldIsRuneSafe(t *testing.T) {
	// Multi-byte characters must never be split; that produces invalid UTF-8
	// which Slack also rejects.
	s := strings.Repeat("é", maxFieldChars+50)
	out := truncateField(s)
	require.True(t, utf8.ValidString(out))
	assert.LessOrEqual(t, utf8.RuneCountInString(out), maxFieldChars)
	assert.True(t, strings.HasSuffix(out, "..."))
	assert.Equal(t, "short", truncateField("short"))
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
