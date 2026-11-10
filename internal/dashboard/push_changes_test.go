package dashboard

import (
	"strings"
	"testing"
)

func pushList(items ...IncidentListItem) pushSnapshot {
	return newPushSnapshot(items)
}

func pushTestItem(id, sev, state string) IncidentListItem {
	return IncidentListItem{ID: id, Severity: sev, State: state,
		Title: "app-" + id + " crash-loops", Namespace: "shop",
		Workload: "app-" + id, WorkloadKind: "Deployment"}
}

func whats(cs []pushChange) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.item.id+":"+c.what)
	}
	return strings.Join(out, " ")
}

func TestPushChangesFollowTheTabRules(t *testing.T) {
	prev := pushList(pushTestItem("a", "notify", "open"),
		pushTestItem("b", "notify", "resolved"),
		pushTestItem("c", "notify", "open"),
		pushTestItem("d", "page", "open"),
		pushTestItem("e", "notify", "open"))
	next := pushList(pushTestItem("a", "notify", "open"),
		pushTestItem("b", "notify", "open"),
		pushTestItem("c", "page", "open"),
		pushTestItem("d", "page", "resolved"),
		pushTestItem("e", "notify", "open"),
		pushTestItem("f", "notify", "settling"),
		pushTestItem("g", "digest", "open"),
		pushTestItem("h", "silent", "open"),
		pushTestItem("i", "notify", "resolved"))
	all := pushPrefs{Min: "digest", Resolved: true}
	if got := whats(pushChanges(prev, next, all)); got !=
		"c:escalated d:resolved b:reopened g:new" {
		t.Errorf("everything: %s", got)
	}
	if got := whats(pushChanges(prev, next,
		pushPrefs{Min: "notify"})); got != "c:escalated b:reopened" {
		t.Errorf("notify, no resolves: %s", got)
	}
	if got := whats(pushChanges(prev, next,
		pushPrefs{Min: "page", Resolved: true})); got !=
		"c:escalated d:resolved" {
		t.Errorf("urgent only: %s", got)
	}
	quiet := pushList(pushTestItem("q", "silent", "open"))
	up := pushMessages(pushChanges(quiet,
		pushList(pushTestItem("q", "notify", "open")), all))
	if len(up) != 1 || !strings.HasSuffix(up[0].Body,
		"Now needs attention (was quiet)") {
		t.Errorf("escalation from quiet: %+v", up)
	}
	if got := pushChanges(nil, next, all); got != nil {
		t.Errorf("first check told %s", whats(got))
	}
	if got := pushChanges(prev, next, pushPrefs{Min: "silent"}); got != nil {
		t.Errorf("min silent told %s", whats(got))
	}
}

func TestPushMessagesReadLikeTheDashboard(t *testing.T) {
	prev := pushList(pushTestItem("a", "notify", "open"),
		pushTestItem("b", "page", "open"))
	cluster := pushTestItem("n", "page", "open")
	cluster.Namespace, cluster.Workload, cluster.WorkloadKind = "", "", ""
	cluster.Title = "Cause now known: node ip-1 is not ready"
	next := pushList(pushTestItem("a", "page", "open"),
		pushTestItem("b", "page", "resolved"), cluster)
	got := pushMessages(pushChanges(prev, next,
		pushPrefs{Min: "notify", Resolved: true}))
	want := []pushMessage{
		{Title: "Urgent: app-a crash-loops",
			Body: "shop · deployment/app-a · Now urgent (was needs attention)",
			Tag:  "a", URL: "/issues/a"},
		{Title: "Resolved: app-b crash-loops",
			Body: "shop · deployment/app-b · Resolved", Tag: "b",
			URL: "/issues/b"},
		{Title: "Urgent: Node ip-1 is not ready",
			Body: "whole cluster · New problem", Tag: "n", URL: "/issues/n"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d = %+v\nwant %+v", i, got[i], want[i])
		}
	}
	var many []IncidentListItem
	for _, id := range []string{"p", "q", "r", "s"} {
		many = append(many, pushTestItem(id, "notify", "open"))
	}
	sum := pushMessages(pushChanges(pushList(), pushList(many...),
		pushPrefs{Min: "notify"}))
	if len(sum) != 1 || sum[0].Title != "4 issues changed" ||
		sum[0].URL != "/issues" {
		t.Errorf("summary = %+v", sum)
	}
	long := pushTestItem("l", "page", "open")
	long.Title = strings.Repeat("é", 500)
	m := pushMessages(pushChanges(pushList(), pushList(long),
		pushPrefs{Min: "page"}))
	if n := len([]rune(m[0].Title)); n != maxPushText {
		t.Errorf("long title is %d runes", n)
	}
	odd := pushTestItem("x/y", "page", "open")
	m = pushMessages(pushChanges(pushList(), pushList(odd),
		pushPrefs{Min: "page"}))
	if m[0].URL != "/issues/x%2Fy" {
		t.Errorf("URL = %s", m[0].URL)
	}
}

func TestProblemTitleMatchesTheDashboard(t *testing.T) {
	for _, c := range []struct{ in, name, want string }{
		{"api crash-loops", "api", "api crash-loops"},
		{"Cause now known: api crash-loops", "api", "api crash-loops"},
		{"Cause now known: two pods fail", "", "Two pods fail"},
		{"Cause now known: ip-10-0 is down", "", "ip-10-0 is down"},
		{"Moved: this is now part of a node outage", "web",
			"web is now part of a node outage"},
		{"Moved: this is now part of a node outage", "",
			"Now part of a node outage"},
		{"Cause now known: ", "", ""},
	} {
		if got := problemTitle(c.in, c.name); got != c.want {
			t.Errorf("problemTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The installed app's icon counts the urgent issues still failing.
func TestPushSnapshotCountsUrgentIssues(t *testing.T) {
	s := pushList(pushTestItem("a", "page", "open"),
		pushTestItem("b", "page", stateResolved),
		pushTestItem("c", "notify", "open"),
		pushTestItem("d", "page", "flapping"))
	if got := s.urgent(); got != 2 {
		t.Errorf("urgent = %d, want 2", got)
	}
}
