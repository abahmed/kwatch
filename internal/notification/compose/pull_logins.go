package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// loginTypes are the Secret types that hold a registry login.
var loginTypes = map[string]bool{
	"kubernetes.io/dockerconfigjson": true,
	"kubernetes.io/dockercfg":        true,
}

// pullLoginSentences say which registry login the failing pods pull
// with, and what the registry answered: "It pulls with Secret regcred
// in ci (kubernetes.io/dockerconfigjson, last changed 92 days ago), and
// the registry answers "unauthorized: authentication required"." Only
// a Secret's name, type and change time are said, never its content.
func pullLoginSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if cause == nil || len(cause.Logins) == 0 {
		return nil
	}
	var items []string
	for _, login := range cause.Logins {
		items = append(items, loginPhrase(login, f.now))
	}
	text := "It pulls with " + strings.Join(items, " and ")
	if len(cause.Logins) == 1 && cause.Logins[0].State == rootcause.LoginNone {
		text = "Its pods name no pull secret, so the registry sees " +
			"anonymous pulls"
	}
	if cause.Refused != "" {
		text += ", and the registry answers " + quoted(cause.Refused)
	}
	return []sentence{{part: partCause,
		text: endSentence(text)}}
}

// loginPhrase names one login: "Secret regcred in ci (type, last
// changed 92 days ago)".
func loginPhrase(l rootcause.PullLogin, now time.Time) string {
	switch l.State {
	case rootcause.LoginNone:
		return "no pull secret"
	case rootcause.LoginMissing:
		return "Secret " + l.Secret + " in " + l.Namespace +
			", which does not exist"
	case rootcause.LoginFound:
		return "Secret " + l.Secret + " in " + l.Namespace + " (" +
			loginDetail(l, now) + ")"
	}
	return "Secret " + l.Secret + " in " + l.Namespace
}

// loginDetail is a found Secret's type and age.
func loginDetail(l rootcause.PullLogin, now time.Time) string {
	var parts []string
	switch {
	case loginTypes[l.Type]:
		parts = append(parts, l.Type)
	case l.Type != "":
		parts = append(parts, "type "+l.Type+", not a registry login")
	}
	if !l.Changed.IsZero() {
		parts = append(parts, "last changed "+ageWords(now.Sub(l.Changed)))
	}
	return strings.Join(parts, ", ")
}

// ageWords counts days from a day on: a login is old by the day, and
// "92 days ago" says more than "13 weeks ago".
func ageWords(d time.Duration) string {
	if d < format.Day {
		return humanDuration(d) + " ago"
	}
	days := int(d / format.Day)
	if days == 1 {
		return "1 day ago"
	}
	return strconv.Itoa(days) + " days ago"
}
