package dashboard

import (
	"cmp"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The browser notifications kwatch pushes follow the same rules as the
// ones an open tab shows (web/js/notify.js): an issue at least as
// severe as the browser asked for that is new, failing again or more
// severe, and its end when the browser asked for that too.

// pushItem is what a push compares of one issue.
type pushItem struct {
	id, severity, title, where string
	resolved                   bool
}

// pushSnapshot is every issue at one check, by ID.
type pushSnapshot map[string]pushItem

// newPushSnapshot takes the list items of one check.
func newPushSnapshot(items []IncidentListItem) pushSnapshot {
	out := make(pushSnapshot, len(items))
	for _, it := range items {
		out[it.ID] = pushItem{id: it.ID, severity: it.Severity,
			title:    problemTitle(it.Title, it.Workload),
			where:    pushWhere(it),
			resolved: it.State == stateResolved}
	}
	return out
}

// pushWhere is the issue's place, as the dashboard writes it:
// "payments · deployment/api", or "whole cluster".
func pushWhere(it IncidentListItem) string {
	where := it.Namespace
	if where == "" {
		where = "whole cluster"
	}
	if it.Workload == "" {
		return where
	}
	obj := it.Workload
	if it.WorkloadKind != "" {
		obj = strings.ToLower(it.WorkloadKind) + "/" + obj
	}
	return where + " · " + obj
}

// problemPrefix starts the titles of updates the dashboard shows as
// the problem itself (KW.problemTitle in web/js/core.js).
var problemPrefix = regexp.MustCompile(`^(Cause now known|Moved):\s*`)

// problemTitle is the title as the dashboard shows it: "Cause now
// known: api crash-loops" reads "api crash-loops". name is the
// issue's object, which keeps its own case.
func problemTitle(t, name string) string {
	m := problemPrefix.FindStringSubmatch(t)
	if m == nil {
		return t
	}
	s := t[len(m[0]):]
	if rest, ok := strings.CutPrefix(s, "this is "); ok &&
		m[1] == "Moved" && strings.HasPrefix(rest, "now part of ") {
		if name != "" {
			return name + " is " + rest
		}
		s = rest
	}
	first, _, _ := strings.Cut(s, " ")
	if s == "" || first == name ||
		strings.ContainsAny(first, "-/.:_0123456789") {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// pushRank orders severities, most urgent first; anything else
// (silent) is never pushed.
var pushRank = map[string]int{"page": 0, "notify": 1, "digest": 2}

// pushLabels are the severity badges of the dashboard (KW.SEVERITY).
var pushLabels = map[string]string{"page": "Urgent",
	"notify": "Needs attention", "digest": "Low", "silent": "Quiet"}

// pushChange is one issue worth telling a browser about.
type pushChange struct {
	what string // new, reopened, escalated or resolved
	item pushItem
	was  string // the severity before an escalation
}

// pushChanges lists what changed from prev to next that a browser with
// prefs wants told, in a stable order.
func pushChanges(prev, next pushSnapshot, prefs pushPrefs) []pushChange {
	limit, ok := pushRank[prefs.Min]
	if prev == nil || !ok {
		return nil
	}
	var out []pushChange
	for _, now := range next {
		rank, ok := pushRank[now.severity]
		if !ok || rank > limit {
			continue
		}
		before, seen := prev[now.id]
		c := pushChange{item: now, was: before.severity}
		switch {
		case now.resolved:
			if !seen || before.resolved || !prefs.Resolved {
				continue
			}
			c.what = "resolved"
		case !seen:
			c.what = "new"
		case before.resolved:
			c.what = "reopened"
		case pushRankOf(before.severity) > rank:
			c.what = "escalated"
		default:
			continue
		}
		out = append(out, c)
	}
	sortChanges(out)
	return out
}

// pushRankOf is pushRank with unknown severities last.
func pushRankOf(sev string) int {
	if r, ok := pushRank[sev]; ok {
		return r
	}
	return len(pushRank)
}

// sortChanges puts the most severe first, then by ID.
func sortChanges(cs []pushChange) {
	slices.SortFunc(cs, func(a, b pushChange) int {
		return cmp.Or(cmp.Compare(pushRankOf(a.item.severity),
			pushRankOf(b.item.severity)), cmp.Compare(a.item.id, b.item.id))
	})
}

// pushBatch is how many changes one check tells one by one; more
// become one summary.
const pushBatch = 3

// pushMessage is one notification as the service worker shows it.
type pushMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Tag   string `json:"tag"`
	URL   string `json:"url"`
}

// maxPushText bounds a title or body in runes, so every message fits
// one push.
const maxPushText = 300

// pushMessages turns changes into notifications.
func pushMessages(cs []pushChange) []pushMessage {
	if len(cs) > pushBatch {
		return []pushMessage{{Title: strconv.Itoa(len(cs)) +
			" issues changed", Body: "Open kwatch to see them.",
			Tag: "kwatch-summary", URL: "/issues"}}
	}
	out := make([]pushMessage, 0, len(cs))
	for _, c := range cs {
		label := pushLabels[c.item.severity]
		head := label
		if c.what == "resolved" {
			head = "Resolved"
		}
		out = append(out, pushMessage{
			Title: cut(head + ": " + c.item.title),
			Body:  cut(c.item.where + " · " + changeWords(c, label)),
			Tag:   c.item.id,
			URL:   "/issues/" + url.PathEscape(c.item.id),
		})
	}
	return out
}

// changeWords says what happened in plain words.
func changeWords(c pushChange, label string) string {
	switch c.what {
	case "new":
		return "New problem"
	case "reopened":
		return "Failing again"
	case "resolved":
		return "Resolved"
	}
	s := "Now " + strings.ToLower(label)
	if was, ok := pushLabels[c.was]; ok {
		s += " (was " + strings.ToLower(was) + ")"
	}
	return s
}

// cut bounds s to maxPushText runes.
func cut(s string) string {
	r := []rune(s)
	if len(r) <= maxPushText {
		return s
	}
	return string(r[:maxPushText-1]) + "…"
}
