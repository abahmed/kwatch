package notification

import (
	"regexp"
	"strings"
)

// zeroWidthSpace breaks a mention so it renders as text but no longer
// notifies anyone.
const zeroWidthSpace = "​"

// broadcastMention matches the channel-wide mentions chat servers expand.
var broadcastMention = regexp.MustCompile(
	`(?i)@(channel|all|here|everyone|room)\b`)

// NeutralizeMentions breaks broadcast mentions with a zero-width space so
// workload log text renders "@channel" without notifying a whole channel.
func NeutralizeMentions(text string) string {
	return broadcastMention.ReplaceAllString(text, "@"+zeroWidthSpace+"$1")
}

// Each chat service has its own mention syntax on top of the plain
// "@channel" style. Workload text must never trigger any of them.
var (
	// Google Chat: <users/all> notifies the whole space, <users/123> a person.
	googleChatMentions = strings.NewReplacer(
		"<users/", "<"+zeroWidthSpace+"users/")
	// Webex: <@all>, <@personEmail:...> and <@personId:...>.
	webexMentions = strings.NewReplacer("<@", "<"+zeroWidthSpace+"@")
	// Zulip: @**all**, @**everyone**, @**Name** and the silent @_**Name**.
	zulipMentions = strings.NewReplacer(
		"@**", "@"+zeroWidthSpace+"**", "@_**", "@"+zeroWidthSpace+"_**")
)

// NeutralizeGoogleChatMentions neutralizes plain and Google Chat mentions.
func NeutralizeGoogleChatMentions(text string) string {
	return googleChatMentions.Replace(NeutralizeMentions(text))
}

// NeutralizeWebexMentions neutralizes plain and Webex Markdown mentions.
func NeutralizeWebexMentions(text string) string {
	return webexMentions.Replace(NeutralizeMentions(text))
}

// NeutralizeZulipMentions neutralizes plain and Zulip mentions.
func NeutralizeZulipMentions(text string) string {
	return zulipMentions.Replace(NeutralizeMentions(text))
}
