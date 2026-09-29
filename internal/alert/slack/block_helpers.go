package slack

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/metrics"

	slackClient "github.com/slack-go/slack"
)

// Slack Block Kit hard limits. Exceeding any one of them makes the API reject
// the entire message with invalid_blocks, so the alert is lost rather than
// degraded. Every limit below is enforced, not assumed.
const (
	maxFieldsPerSection = 10
	maxFieldChars       = 2000
	maxBlocksPerMessage = 50
)

// truncateField shortens s to Slack's per-field limit. It slices on rune
// boundaries so a multi-byte character is never split into invalid UTF-8.
func truncateField(s string) string {
	const maxChars = maxFieldChars
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	const ellipsis = "..."
	if maxChars <= len(ellipsis) {
		return string(r[:maxChars])
	}
	return string(r[:maxChars-len(ellipsis)]) + ellipsis
}

// capBlocks keeps a message within Slack's block limit, reserving the last
// slot for a marker so a trimmed alert says so instead of quietly dropping
// evidence.
func capBlocks(blocks []slackClient.Block) []slackClient.Block {
	if len(blocks) <= maxBlocksPerMessage {
		return blocks
	}
	kept := blocks[:maxBlocksPerMessage-1]
	omitted := len(blocks) - len(kept)
	metrics.DefaultRegistry().RenderedDetailsOmitted.Add(int64(omitted))
	return append(kept, markdownSection(
		fmt.Sprintf(
			"_%d more block(s) omitted to stay within Slack's limit._",
			omitted,
		),
	))
}

func plainSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.PlainTextType,
			txt,
			true,
			false),
	}
}

func markdownSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.MarkdownType,
			escapeMrkdwn(txt),
			false,
			true),
	}
}

func markdownF(format string, a ...interface{}) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(
		slackClient.MarkdownType,
		escapeMrkdwn(truncateField(fmt.Sprintf(format, a...))),
		false,
		true)
}

// mrkdwnEscaper escapes the three characters Slack treats as control
// sequences. Event text such as "<!channel>" or "<@U123>" from workload logs
// must render literally instead of notifying people.
var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeMrkdwn(text string) string {
	return mrkdwnEscaper.Replace(text)
}
