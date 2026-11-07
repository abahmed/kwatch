package notification

import (
	"regexp"
	"strings"
)

// zeroWidthSpace breaks a mention so it renders as text but no longer
// notifies anyone.
const zeroWidthSpace = "\u200b"

// broadcastMention matches the channel-wide mentions chat servers expand.
// The @ must not follow a word character, so an e-mail address, a URL's
// user part or a DSN such as "app@everyone-db" is left whole.
var broadcastMention = regexp.MustCompile(
	`(?i)(^|[^A-Za-z0-9_])@(channel|all|here|everyone|room)\b`)

// userMention matches any @name mention; as above, an @ inside a word is
// not one.
var userMention = regexp.MustCompile(`(^|[^A-Za-z0-9_])@([A-Za-z0-9_])`)

// slackSpecial matches Slack's <!channel>, <!here>, <!everyone> and
// <!subteam^ID> forms, and its <@U123> user and <#C1> channel references,
// which Slack and the Slack-compatible webhooks of Mattermost and
// Rocket.Chat expand.
var slackSpecial = regexp.MustCompile(`<(![A-Za-z]|[@#])`)

// NeutralizeMentions breaks broadcast mentions with a zero-width space so
// workload log text renders "@channel" and "<!here>" without notifying a
// whole channel.
func NeutralizeMentions(text string) string {
	text = slackSpecial.ReplaceAllString(text, "<"+zeroWidthSpace+"$1")
	return broadcastMention.ReplaceAllString(text,
		"${1}@"+zeroWidthSpace+"${2}")
}

// NeutralizeUserMentions breaks every @name mention as well as the
// broadcast and Slack forms. Mattermost and Rocket.Chat notify the user
// or group of any @name, so "paged @alice and @devops-team" in workload
// text must not reach them.
func NeutralizeUserMentions(text string) string {
	text = slackSpecial.ReplaceAllString(text, "<"+zeroWidthSpace+"$1")
	return userMention.ReplaceAllString(text,
		"${1}@"+zeroWidthSpace+"${2}")
}

// Each chat service has its own mention syntax on top of the plain
// "@channel" style. Workload text must never trigger any of them.
var (
	// Google Chat: <users/all> notifies the whole space, <users/123> a person.
	googleChatMentions = strings.NewReplacer(
		"<users/", "<"+zeroWidthSpace+"users/")
	// Webex: <@all>, <@personEmail:...> and <@personId:...>.
	webexMentions = strings.NewReplacer("<@", "<"+zeroWidthSpace+"@")
)

// NeutralizeGoogleChatMentions neutralizes plain and Google Chat mentions.
func NeutralizeGoogleChatMentions(text string) string {
	return googleChatMentions.Replace(NeutralizeMentions(text))
}

// NeutralizeWebexMentions neutralizes plain and Webex Markdown mentions.
func NeutralizeWebexMentions(text string) string {
	return webexMentions.Replace(NeutralizeMentions(text))
}

// telegramAt matches Telegram's @username mention: a username starts with
// a letter and has 5 to 32 letters, digits and underscores. The @ must
// not follow a word character, so an e-mail address is left whole.
var telegramAt = regexp.MustCompile(
	`(^|[^0-9A-Za-z_])@([A-Za-z][0-9A-Za-z_]{4,31})`)

// NeutralizeTelegramMentions neutralizes plain broadcast mentions and
// Telegram @username mentions, which notify the person or channel that
// owns the name.
func NeutralizeTelegramMentions(text string) string {
	return telegramAt.ReplaceAllString(NeutralizeMentions(text),
		"$1@"+zeroWidthSpace+"$2")
}
