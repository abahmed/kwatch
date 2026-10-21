package notification

import (
	"html"
	"strings"
)

// oneLine folds text onto one line, for markup that cannot span lines.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// foldLines puts text on one line for a code span: a line break becomes
// a space. Runs of spaces are kept, because a code span shows what a pod
// wrote and a pod's own spacing is part of it.
func foldLines(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// backtickRun is the longest run of backticks in s.
func backtickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r != '`' {
			run = 0
			continue
		}
		run++
		if run > longest {
			longest = run
		}
	}
	return longest
}

// fenceOf wraps body in a fence longer than any backtick run in it.
func fenceOf(body string) string {
	n := backtickRun(body) + 1
	if n < 3 {
		n = 3
	}
	f := strings.Repeat("`", n)
	return f + "\n" + body + "\n" + f
}

// codeSpanOf is a CommonMark code span longer than any run inside s. A
// backtick or a space at either edge is padded with a space, which
// CommonMark strips again, so the text shows exactly as written.
func codeSpanOf(s string) string {
	s = foldLines(s)
	f := strings.Repeat("`", backtickRun(s)+1)
	if edgeIs(s, "` ") {
		s = " " + s + " "
	}
	return f + s + f
}

// edgeIs reports whether s starts or ends with one of chars.
func edgeIs(s, chars string) bool {
	return s != "" && (strings.ContainsRune(chars, rune(s[0])) ||
		strings.ContainsRune(chars, rune(s[len(s)-1])))
}

// stripped removes characters from s.
func stripped(s string, chars string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(chars, r) {
			return -1
		}
		return r
	}, s)
}

func identity(s string) string { return s }

// wrapper returns a func that writes s on one line between marks,
// after escape.
func wrapper(mark string, escape func(string) string) func(string) string {
	return func(s string) string {
		return mark + escape(oneLine(s)) + mark
	}
}

// PlainDialect writes the lines with "-" bullets and no markup.
func PlainDialect() Dialect {
	return Dialect{Text: identity, Bold: identity, Italic: identity,
		Code: foldLines, Fence: identity, Bullet: "- ", Gap: "\n"}
}

// Plain writes the message as plain lines.
func (m Message) Plain() string { return m.Render(PlainDialect()) }

// slackEscaper escapes the three characters Slack treats as control
// sequences.
var slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// slackMarked writes bold or italic for Slack, which has no escape for
// its own marker: text that holds the marker is written plain, never with
// the marker dropped, so a name such as my_service stays whole.
func slackMarked(mark string) func(string) string {
	return func(s string) string {
		s = slackEscaper.Replace(oneLine(s))
		if strings.Contains(s, mark) {
			return s
		}
		return mark + s + mark
	}
}

// slackFence breaks a fence inside s so it cannot end the block.
func slackFence(s string) string {
	return "```\n" + slackEscaper.Replace(
		strings.ReplaceAll(s, "```", "``\u200b`")) + "\n```"
}

// SlackDialect writes Slack mrkdwn. Slack cannot escape its markers:
// bold or italic text that holds its marker is written plain, and a
// backtick inside an inline code span becomes ˋ (U+02CB). Text that must
// carry backticks belongs in a code block, which the composer uses.
func SlackDialect() Dialect {
	return Dialect{
		Neutralize: NeutralizeMentions,
		Text:       slackEscaper.Replace,
		Bold:       slackMarked("*"),
		Italic:     slackMarked("_"),
		Code: func(s string) string {
			return "`" + slackEscaper.Replace(
				strings.ReplaceAll(foldLines(s), "`", "ˋ")) + "`"
		},
		Fence:  slackFence,
		Bullet: "• ", Gap: "\n", Ellipsis: "...",
	}
}

// markdownEscaper escapes what CommonMark and its chat dialects treat
// as markup inside running text.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
	"<", `\<`, ">", `\>`, "|", `\|`, "~", `\~`, "#", `\#`)

// MarkdownDialect writes CommonMark: Discord, Mattermost, Rocket.Chat,
// Teams, Zulip, Webex, GitHub, GitLab and Gitea. neutralize may be nil.
func MarkdownDialect(neutralize func(string) string, gap string) Dialect {
	if neutralize == nil {
		neutralize = NeutralizeMentions
	}
	return Dialect{
		Neutralize: neutralize,
		Text:       markdownEscaper.Replace,
		Bold:       wrapper("**", markdownEscaper.Replace),
		Italic:     wrapper("*", markdownEscaper.Replace),
		Code:       codeSpanOf,
		Fence:      fenceOf,
		Bullet:     "- ", Gap: gap,
	}
}

// googleChatMarked writes bold or italic for Google Chat, which has no
// escape for its markers: text that holds the marker is written plain.
func googleChatMarked(mark string) func(string) string {
	return func(s string) string {
		s = oneLine(s)
		if strings.Contains(s, mark) {
			return s
		}
		return mark + s + mark
	}
}

// GoogleChatDialect writes Google Chat's text formatting.
func GoogleChatDialect() Dialect {
	return Dialect{
		Neutralize: NeutralizeGoogleChatMentions,
		Text:       identity,
		Bold:       googleChatMarked("*"),
		Italic:     googleChatMarked("_"),
		Code: func(s string) string {
			return "`" + strings.ReplaceAll(foldLines(s), "`", "ˋ") + "`"
		},
		Fence: func(s string) string {
			return "```\n" + strings.ReplaceAll(s, "```", "``\u200b`") +
				"\n```"
		},
		Bullet: "• ", Gap: "\n",
	}
}

// htmlTag writes text escaped between <name> and </name>. fold puts the
// text on one line.
func htmlTag(name string, fold func(string) string) func(string) string {
	return func(s string) string {
		return "<" + name + ">" + html.EscapeString(fold(s)) +
			"</" + name + ">"
	}
}

// HTMLDialect writes the HTML Telegram, Matrix and mail accept: b, i,
// code and pre, with line breaks as newlines (br when br is set).
func HTMLDialect(neutralize func(string) string, br bool) Dialect {
	if neutralize == nil {
		neutralize = NeutralizeMentions
	}
	gap := "\n"
	if br {
		gap = "<br>\n"
	}
	return Dialect{
		Neutralize: neutralize, Text: html.EscapeString,
		Bold: htmlTag("b", oneLine), Italic: htmlTag("i", oneLine),
		Code: htmlTag("code", foldLines),
		Fence: func(s string) string {
			return "<pre>" + html.EscapeString(s) + "</pre>"
		},
		Bullet: "• ", Gap: gap,
	}
}

// jiraEscaper escapes Jira wiki markup in running text.
var jiraEscaper = strings.NewReplacer(
	`\`, `\\`, "{", `\{`, "}", `\}`, "[", `\[`, "]", `\]`, "*", `\*`,
	"_", `\_`, "|", `\|`, "^", `\^`, "~", `\~`, "!", `\!`, "+", `\+`)

// JiraDialect writes Jira wiki markup.
func JiraDialect() Dialect {
	return Dialect{
		Neutralize: NeutralizeMentions,
		Text:       jiraEscaper.Replace,
		Bold:       wrapper("*", jiraEscaper.Replace),
		Italic:     wrapper("_", jiraEscaper.Replace),
		// A brace cannot be escaped inside a span, so text with one is
		// written as a block (see SpanFits) and never loses a byte.
		SpanFits: func(s string) bool {
			return !strings.ContainsAny(s, "{}")
		},
		Code: func(s string) string {
			// Wiki markup still acts inside {{ }}: [~user] mentions,
			// *bold* and _italic_ all apply, so the text is escaped.
			// Text with a brace never reaches here through Render; a
			// caller that asks for a span anyway gets it without them.
			return "{{" +
				jiraEscaper.Replace(stripped(foldLines(s), "{}")) + "}}"
		},
		Fence: func(s string) string {
			return "{noformat}\n" +
				strings.ReplaceAll(s, "{noformat}", "{ noformat}") +
				"\n{noformat}"
		},
		Bullet: "* ", Gap: "\n",
	}
}
