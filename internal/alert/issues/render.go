package issues

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// Title is the issue title: the Short lead, cut to the tracker's limit.
func Title(msg notification.Message, limit int) string {
	return notification.Truncate(msg.ShortText(), limit)
}

// Body is the issue body or comment: the narrative Note, then the recent
// application output as a fenced block, so the status marker stays the
// first character. The cluster is not added as a label line: the
// composer names it in the Note's sentences.
func Body(msg notification.Message) string {
	return FencedBody(msg, "```")
}

// FencedBody is Body with a tracker-specific code fence, such as Jira's
// "{noformat}". The output is workload text, so it must not be able to
// close the fence early: a backtick fence is made longer than any backtick
// run in the output, and any other fence token is broken up where it
// appears in the output.
func FencedBody(msg notification.Message, fence string) string {
	return FencedBodyWith(msg, fence, nil)
}

// FencedBodyWith is FencedBody with an extra escape applied to the Note
// (for example Jira wiki markup). The Note can carry pod-controlled text
// such as a termination message, so every tracker neutralises @mentions
// in it; output stays verbatim inside its fence, where mentions are inert.
func FencedBodyWith(
	msg notification.Message, fence string, escape func(string) string,
) string {
	note := NeutralizeMentions(msg.NoteText())
	if escape != nil {
		note = escape(note)
	}
	var b strings.Builder
	b.WriteString(note)
	if len(msg.Output) > 0 {
		text := strings.Join(msg.Output, "\n")
		fence, text = safeFence(fence, text)
		b.WriteString("\n\n" + fence + "\n")
		b.WriteString(text)
		b.WriteString("\n" + fence)
	}
	return b.String()
}

// mention matches an @name or @org/team that trackers turn into a
// notification.
// An @ inside a word, as in an email address, is not a mention.
var mention = regexp.MustCompile(`(^|[^A-Za-z0-9_])@([A-Za-z0-9_])`)

// NeutralizeMentions breaks every @mention with a zero-width space after
// the @, so workload text cannot notify a user or a team.
func NeutralizeMentions(text string) string {
	return mention.ReplaceAllString(text, "${1}@"+zeroWidthSpace+"${2}")
}

// zeroWidthSpace splits a fence token without changing how the text reads.
const zeroWidthSpace = "\u200b"

// safeFence returns a fence and content that cannot break out of it.
func safeFence(fence, text string) (string, string) {
	if strings.Trim(fence, "`") == "" {
		longest := longestRun(text, '`')
		if longest >= len(fence) {
			fence = strings.Repeat("`", longest+1)
		}
		return fence, text
	}
	broken := fence[:1] + zeroWidthSpace + fence[1:]
	return fence, strings.ReplaceAll(text, fence, broken)
}

// longestRun is the length of the longest run of c in text.
func longestRun(text string, c byte) int {
	longest, run := 0, 0
	for i := 0; i < len(text); i++ {
		if text[i] != c {
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
