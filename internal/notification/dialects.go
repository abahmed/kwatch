package notification

import (
	"html"
	"strings"
)

// oneLine folds text onto one line, for markup that cannot span lines.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

// codeSpanOf is a CommonMark code span longer than any run inside s.
func codeSpanOf(s string) string {
	s = oneLine(s)
	f := strings.Repeat("`", backtickRun(s)+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return f + s + f
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
		Code: oneLine, Fence: identity, Bullet: "- ", Gap: "\n"}
}

// Plain writes the message as plain lines.
func (m Message) Plain() string { return m.Render(PlainDialect()) }

// slackEscaper escapes the three characters Slack treats as control
// sequences.
var slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// slackMarked writes bold or italic for Slack, which has no escape for
// its own marker: the marker inside the text is dropped.
func slackMarked(mark string) func(string) string {
	return func(s string) string {
		return mark + slackEscaper.Replace(stripped(oneLine(s), mark)) + mark
	}
}

// slackFence breaks a fence inside s so it cannot end the block.
func slackFence(s string) string {
	return "```\n" + slackEscaper.Replace(
		strings.ReplaceAll(s, "```", "``\u200b`")) + "\n```"
}

// SlackDialect writes Slack mrkdwn. Markers inside bold and code are
// replaced, as Slack cannot escape them.
func SlackDialect() Dialect {
	return Dialect{
		Neutralize: NeutralizeMentions,
		Text:       slackEscaper.Replace,
		Bold:       slackMarked("*"),
		Italic:     slackMarked("_"),
		Code: func(s string) string {
			return "`" + slackEscaper.Replace(
				strings.ReplaceAll(oneLine(s), "`", "ˋ")) + "`"
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
// escape for its markers.
func googleChatMarked(mark string) func(string) string {
	return func(s string) string {
		return mark + stripped(oneLine(s), mark) + mark
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
			return "`" + strings.ReplaceAll(oneLine(s), "`", "ˋ") + "`"
		},
		Fence: func(s string) string {
			return "```\n" + strings.ReplaceAll(s, "```", "``\u200b`") +
				"\n```"
		},
		Bullet: "• ", Gap: "\n",
	}
}

// htmlTag writes text escaped between <name> and </name>.
func htmlTag(name string) func(string) string {
	return func(s string) string {
		return "<" + name + ">" + html.EscapeString(oneLine(s)) +
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
		Bold: htmlTag("b"), Italic: htmlTag("i"), Code: htmlTag("code"),
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
		Code: func(s string) string {
			return "{{" + stripped(oneLine(s), "{}") + "}}"
		},
		Fence: func(s string) string {
			return "{noformat}\n" +
				strings.ReplaceAll(s, "{noformat}", "{ noformat}") +
				"\n{noformat}"
		},
		Bullet: "* ", Gap: "\n",
	}
}
