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
	maxSectionTextChars = 3000
	maxBlocksPerMessage = 50
)

// truncateMrkdwn shortens already escaped mrkdwn to maxChars characters.
// It counts runes, so a multi-byte character is never split into invalid
// UTF-8, and it never cuts an escape such as "&amp;" in half, which would
// show a broken entity. Text must be escaped before it is cut: escaping
// after cutting can push the text past the limit again.
func truncateMrkdwn(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	const ellipsis = "..."
	if maxChars <= len(ellipsis) {
		return string(r[:maxChars])
	}
	kept := string(r[:maxChars-len(ellipsis)])
	if amp := strings.LastIndexByte(kept, '&'); amp >= 0 &&
		!strings.Contains(kept[amp:], ";") {
		kept = kept[:amp]
	}
	return kept + ellipsis
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

// markdownSection escapes txt and then cuts it to the section text limit.
func markdownSection(txt string) slackClient.SectionBlock {
	return escapedSection(
		truncateMrkdwn(escapeMrkdwn(txt), maxSectionTextChars))
}

// codeSection shows txt as a code block. The text is cut before the fences
// are added, so the closing fence is never lost.
func codeSection(txt string) slackClient.SectionBlock {
	const fence = "```"
	// Output is workload text: break any fence in it so it cannot end the
	// block early and turn the rest into live mrkdwn.
	txt = strings.ReplaceAll(txt, fence, "``\u200b`")
	body := truncateMrkdwn(escapeMrkdwn(txt),
		maxSectionTextChars-2*len(fence))
	return escapedSection(fence + body + fence)
}

func escapedSection(escaped string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.MarkdownType, escaped, false, true),
	}
}

// mrkdwnEscaper escapes the three characters Slack treats as control
// sequences. Event text such as "<!channel>" or "<@U123>" from workload logs
// must render literally instead of notifying people.
var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeMrkdwn(text string) string {
	return mrkdwnEscaper.Replace(text)
}
