package providertest

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

// MentionText is quoted cluster text that would notify a whole room on
// most chat services if it reached the provider untouched.
const MentionText = "@channel @everyone <!here> @all <@U123> " +
	"<@W456> <#C1|general>"

// massMention matches the broadcast forms chat services expand.
var massMention = regexp.MustCompile(`(?i)@(channel|all|here|everyone|` +
	`room)\b|<!(channel|here|everyone|subteam)|<@all>|<users/all>|` +
	`@\*\*(all|everyone)\*\*|@_?\*\*`)

// Mentions is an incident whose note, headline, blocks and output all
// quote MentionText, as a pod log line would.
func Mentions() notification.Message {
	m := withKey(Announce(), "p-mention")
	quote := "It said \"" + MentionText + "\"."
	m.Short = "🔴 payments in shop is crash-looping. " + quote
	m.Note = m.Short + " Run kubectl logs payments -n shop"
	m.Output = []string{"log: " + MentionText}
	m.Doc = []notification.Block{
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: "🔴 payments in shop is crash-looping. " + quote}}},
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: "It said "},
			{Text: MentionText, Style: notification.Code}}},
		{Kind: notification.CodeBlock, Spans: []notification.Span{
			{Text: MentionText}}},
	}
	return m
}

// AssertNoMassMention fails when a request carries a live broadcast
// mention, looking at the decoded text of its JSON or form body.
func AssertNoMassMention(t *testing.T, reqs []Request) {
	t.Helper()
	for _, r := range reqs {
		for _, s := range visibleStrings(r) {
			if m := massMention.FindString(s); m != "" {
				t.Fatalf("live mention %q in %s %s: %.300q",
					m, r.Method, r.Path, s)
			}
		}
	}
}

// visibleStrings is the raw body plus every string the body decodes to,
// through form fields and nested JSON.
func visibleStrings(r Request) []string {
	out := []string{string(r.Body)}
	if form, err := url.ParseQuery(string(r.Body)); err == nil {
		for _, values := range form {
			for _, v := range values {
				out = append(out, v)
				out = append(out, jsonStrings([]byte(v))...)
			}
		}
	}
	return append(out, jsonStrings(r.Body)...)
}

func jsonStrings(raw []byte) []string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			out = append(out, x)
			if strings.HasPrefix(strings.TrimSpace(x), "{") ||
				strings.HasPrefix(strings.TrimSpace(x), "[") {
				out = append(out, jsonStrings([]byte(x))...)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}
