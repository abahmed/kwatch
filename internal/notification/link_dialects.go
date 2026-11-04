package notification

import (
	"html"
	"strings"
)

// slackLink writes Slack's <address|label>. The address has no | or >
// (linkURL encodes them); & < > are entities in both parts, the way
// Slack reads every control character.
func slackLink(text, address string) string {
	return "<" + slackEscaper.Replace(address) + "|" +
		slackEscaper.Replace(oneLine(text)) + ">"
}

// linkTextEscaper escapes a Markdown link label: the running-text
// escapes, and the parens CommonMark would read as the destination.
var linkTextEscaper = strings.NewReplacer("(", `\(`, ")", `\)`)

// markdownLink writes CommonMark's [label](address). The address has no
// space, paren or angle bracket left (linkURL encodes them).
func markdownLink(text, address string) string {
	return "[" + linkTextEscaper.Replace(
		markdownEscaper.Replace(oneLine(text))) + "](" + address + ")"
}

// googleChatLink writes Google Chat's <address|label>. Google Chat has
// no escapes, so a label holding < > or | is written as plain words.
func googleChatLink(text, address string) string {
	text = oneLine(text)
	if strings.ContainsAny(text, "<>|") {
		return linkWords(text, address)
	}
	return "<" + address + "|" + text + ">"
}

// htmlLink writes <a href="address">label</a>, both escaped.
func htmlLink(text, address string) string {
	return `<a href="` + html.EscapeString(address) + `">` +
		html.EscapeString(oneLine(text)) + "</a>"
}

// jiraLink writes Jira wiki markup's [label|address]. The address has
// no | or ] left (linkURL encodes them).
func jiraLink(text, address string) string {
	return "[" + jiraEscaper.Replace(oneLine(text)) + "|" + address + "]"
}
