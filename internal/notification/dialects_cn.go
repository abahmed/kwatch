package notification

import (
	"regexp"
	"strings"
)

// feishuTags matches Feishu's <at ...> mention and <font ...> tags,
// which card markdown acts on.
var feishuTags = regexp.MustCompile(`(?i)<(/?)(at|font|text_tag)\b`)

// NeutralizeFeishuMentions neutralizes plain broadcast mentions and
// Feishu's <at id=all></at> and <at user_id=...> tags.
func NeutralizeFeishuMentions(text string) string {
	return feishuTags.ReplaceAllString(NeutralizeMentions(text),
		"<$1"+zeroWidthSpace+"$2")
}

// FeishuDialect writes the Markdown subset of a Feishu card's markdown
// element: bold, italic, inline code, code blocks and "-" lists.
func FeishuDialect() Dialect {
	return MarkdownDialect(NeutralizeFeishuMentions, "\n\n")
}

// dingTalkAt matches DingTalk's @phone and @userId mentions, which a
// markdown text turns into an @ of that person. The @ must not follow a
// word character, so an e-mail address is left whole.
var dingTalkAt = regexp.MustCompile(`(^|[^0-9A-Za-z._-])@([0-9A-Za-z])`)

// NeutralizeDingTalkMentions neutralizes plain broadcast mentions and
// DingTalk's @phone-number and @id forms.
func NeutralizeDingTalkMentions(text string) string {
	return dingTalkAt.ReplaceAllString(NeutralizeMentions(text),
		"$1@"+zeroWidthSpace+"$2")
}

// dingTalkQuote writes text as a block quote, the only way DingTalk's
// markdown subset sets verbatim text apart: it has no code blocks.
func dingTalkQuote(s string) string {
	lines := strings.Split(markdownEscaper.Replace(s), "\n")
	return "> " + strings.Join(lines, "\n> ")
}

// DingTalkDialect writes the Markdown subset DingTalk robots render:
// headings, bold, italic, quotes and lists. Code has no markup there, so
// a code span is escaped plain text and a code block is a quote.
func DingTalkDialect() Dialect {
	d := MarkdownDialect(NeutralizeDingTalkMentions, "\n\n")
	d.Code = func(s string) string {
		return markdownEscaper.Replace(foldLines(s))
	}
	d.Fence = dingTalkQuote
	return d
}
