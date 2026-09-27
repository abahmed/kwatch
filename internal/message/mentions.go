package message

import "regexp"

// broadcastMention matches the channel-wide mentions chat servers expand.
var broadcastMention = regexp.MustCompile(`(?i)@(channel|all|here|everyone)\b`)

// NeutralizeMentions breaks broadcast mentions with a zero-width space so
// workload log text renders "@channel" without notifying a whole channel.
func NeutralizeMentions(text string) string {
	return broadcastMention.ReplaceAllString(text, "@​$1")
}
