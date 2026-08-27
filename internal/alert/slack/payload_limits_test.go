package slack

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
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

func hostileIncident(events int, msgLen int) *model.Incident {
	var ev strings.Builder
	for i := 0; i < events; i++ {
		fmt.Fprintf(
			&ev,
			"Aug 25 23:%02d:00  FailedScheduling  0/7 nodes are available: 7 "+
				"node(s) had untolerated taint(s). preemption: 0/7 nodes are "+
				"available.\n",
			i%60,
		)
	}
	return &model.Incident{
		Subject: model.Subject{
			Name:          "api",
			Reason:        "Error",
			Namespace:     "dev",
			OwnerKind:     "Deployment",
			ContainerName: "api",
			Image: "registry.example.com/team/api:" +
				"1.2.0",
			NodeName: "ip-10-0-81-7.us-east-1.compute.internal",
		},
		Status: model.Status{
			Count:         7,
			RestartCount:  3,
			PeakResources: 4,
			Resources: map[string]bool{
				"a": true,
				"b": true,
				"c": true,
				"d": true,
			},
			LastContainerState: &model.ContainerState{
				Msg:      strings.Repeat("x", msgLen),
				ExitCode: 137,
			},
		},
		Evidence: model.Evidence{
			Hint:          strings.Repeat("hint ", msgLen/5),
			Runbook:       "https://runbooks.example.com/error",
			Events:        ev.String(),
			IncludeEvents: true,
			Logs:          strings.Repeat("log line\n", 400),
			IncludeLogs:   true,
		},
	}

}

func TestSlackPayloadStaysWithinEveryLimit(t *testing.T) {
	app := &config.App{ClusterName: "dev"}
	for _, tc := range []struct {
		name   string
		events int
		msgLen int
	}{
		{"typical", 20, 60},
		{"busy pod", 400, 60},
		{"long kubernetes message", 20, 5000},
		{"pathological", 5000, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inc := hostileIncident(tc.events, tc.msgLen)
			for _, b := range []*slackClient.Blocks{
				buildIncidentBlocksWithInsight(inc, app, nil),
				buildIncidentUpdateBlocks(inc),
				buildIncidentResolvedBlocks(inc),
			} {
				fields, fieldChars, sectionChars, blocks := payloadStats(b)
				assert.LessOrEqual(
					t,
					fields,
					maxFieldsPerSection,
					"fields per section",
				)
				assert.LessOrEqual(
					t,
					fieldChars,
					maxFieldChars,
					"chars per field",
				)
				assert.LessOrEqual(
					t,
					sectionChars,
					3000,
					"chars per section text",
				)
				assert.LessOrEqual(
					t,
					blocks,
					maxBlocksPerMessage,
					"blocks per message",
				)
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
