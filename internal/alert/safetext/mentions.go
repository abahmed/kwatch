package safetext

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// zeroWidthSpace breaks a mention so it shows as text but notifies nobody.
const zeroWidthSpace = "\u200b"

// weComMentions covers WeCom's <@userid> syntax.
var weComMentions = strings.NewReplacer("<@", "<"+zeroWidthSpace+"@")

// zulipMentions covers every Zulip mention form: @**user**, the silent
// @_**user**, and the user-group forms @*group* and @_*group*.
var zulipMentions = strings.NewReplacer(
	"@_*", "@"+zeroWidthSpace+"_*",
	"@*", "@"+zeroWidthSpace+"*",
)

// matrixUserID matches @localpart:server, the form Matrix push rules and
// clients turn into a mention of that user.
var matrixUserID = regexp.MustCompile(
	`@([A-Za-z0-9._=/+-]+:[A-Za-z0-9.-]+)`)

// Matrix neutralizes @room and every other broadcast mention, and breaks
// each @user:server so workload text cannot ping a person.
func Matrix(text string) string {
	return matrixUserID.ReplaceAllString(
		notification.NeutralizeMentions(text), "@"+zeroWidthSpace+"$1")
}

// WeCom neutralizes plain broadcast mentions and <@userid>.
func WeCom(text string) string {
	return weComMentions.Replace(notification.NeutralizeMentions(text))
}

// Zulip neutralizes plain broadcast mentions and every Zulip mention,
// including user groups.
func Zulip(text string) string {
	return zulipMentions.Replace(notification.NeutralizeMentions(text))
}

// Lines neutralizes each line of output with fn.
func Lines(output []string, fn func(string) string) []string {
	out := make([]string, len(output))
	for i, line := range output {
		out[i] = fn(line)
	}
	return out
}
