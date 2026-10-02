package issues

import (
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
// "{noformat}".
func FencedBody(msg notification.Message, fence string) string {
	var b strings.Builder
	b.WriteString(msg.NoteText())
	if len(msg.Output) > 0 {
		b.WriteString("\n\n" + fence + "\n")
		b.WriteString(strings.Join(msg.Output, "\n"))
		b.WriteString("\n" + fence)
	}
	return b.String()
}
