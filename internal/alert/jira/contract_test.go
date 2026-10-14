package jira

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const (
	testToken = "jira-secret-token"
	// createdKey is the issue key the recorder answers a create with.
	createdKey = "OPS-9"
	apiBase    = "/rest/api/2"
)

var issueRef = regexp.MustCompile(`/issue/([A-Z]+-\d+)`)

// TestContract pins the exact Jira REST v2 requests: the JQL search, the
// create, the comment and the closing transition.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.Path, "/search"):
			_, _ = w.Write([]byte(`{"issues":[]}`))
		case strings.HasSuffix(r.Path, "/transitions") &&
			r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"transitions":[` +
				`{"id":"31","name":"Done","to":{"name":"Done"}}]}`))
		case r.Method == http.MethodPost && r.Path == apiBase+"/issue":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"` + createdKey + `"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}
	j := NewJira(map[string]interface{}{
		"url": rec.URL(), "user": "ops@example.com",
		"apiToken": testToken, "projectKey": "OPS",
		"closeTransition": "Done",
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: j.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{testToken},
		KeyOf:   issueKey,
		Check:   checkIssueShape,
	})
}

// issueKey is the issue a request addresses; a create addresses the one
// the recorder numbered, so every lifecycle step lands on one issue.
func issueKey(t *testing.T, r providertest.Request) string {
	if m := issueRef.FindStringSubmatch(r.Path); m != nil {
		return m[1]
	}
	if r.Method == http.MethodPost {
		return createdKey
	}
	t.Fatalf("request addresses no issue: %s %s", r.Method, r.Path)
	return ""
}

func checkIssueShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	for _, r := range reqs {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
	}
	issue := apiBase + "/issue/" + createdKey
	switch name {
	case "digest":
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an issue")
		}
	case "notify-update":
		if len(reqs) != 1 {
			t.Fatalf("update sent %d requests", len(reqs))
		}
		checkComment(t, reqs[0], issue+"/comment")
	case "resolve":
		checkResolve(t, reqs, issue)
	default:
		checkCreate(t, name, reqs)
	}
}

func checkResolve(t *testing.T, reqs []providertest.Request, issue string) {
	if len(reqs) != 3 {
		t.Fatalf("resolve sent %d requests", len(reqs))
	}
	checkComment(t, reqs[0], issue+"/comment")
	list, apply := reqs[1], reqs[2]
	tr, _ := apply.JSON(t)["transition"].(map[string]any)
	if list.Method != "GET" || list.Path != issue+"/transitions" ||
		apply.Method != "POST" || apply.Path != issue+"/transitions" ||
		tr["id"] != "31" {
		t.Fatalf("resolve must apply the Done transition: %+v", reqs)
	}
}

func checkComment(t *testing.T, r providertest.Request, path string) {
	body, _ := r.JSON(t)["body"].(string)
	if r.Method != "POST" || r.Path != path || body == "" {
		t.Fatalf("comment = %s %s %q", r.Method, r.Path, body)
	}
	checkWiki(t, body)
}

func checkCreate(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 2 {
		t.Fatalf("create sent %d requests, want search and create",
			len(reqs))
	}
	search, create := reqs[0], reqs[1]
	jql, _ := url.ParseQuery(search.Query)
	if search.Method != "GET" || search.Path != apiBase+"/search" ||
		!strings.Contains(jql.Get("jql"), `project = "OPS"`) ||
		!strings.Contains(jql.Get("jql"), "kwatchkey") ||
		!strings.Contains(jql.Get("jql"), "statusCategory != Done") {
		t.Fatalf("search = %s %s?%s", search.Method, search.Path,
			search.Query)
	}
	fields, _ := create.JSON(t)["fields"].(map[string]any)
	project, _ := fields["project"].(map[string]any)
	kind, _ := fields["issuetype"].(map[string]any)
	summary, _ := fields["summary"].(string)
	desc, _ := fields["description"].(string)
	if create.Method != "POST" || create.Path != apiBase+"/issue" ||
		project["key"] != "OPS" || kind["name"] != "Task" ||
		summary == "" {
		t.Fatalf("create = %s %s %v", create.Method, create.Path, fields)
	}
	if n := utf8.RuneCountInString(summary); n > titleLimit {
		t.Fatalf("summary is %d runes, limit %d", n, titleLimit)
	}
	providertest.AssertOneLeadingEmoji(t, summary)
	if !strings.Contains(desc, "Ref: kwatchkey") {
		t.Fatal("description lacks the idempotency marker")
	}
	checkWiki(t, desc)
	if name == "markdown-code" &&
		!strings.Contains(desc, "{{boom \\*x\\* @\u200bchannel}}") {
		t.Fatalf("code span does not escape * and _: %q", desc)
	}
}

// checkWiki requires Jira wiki markup: no Markdown fences, and every
// {noformat} block closed.
func checkWiki(t *testing.T, text string) {
	if strings.Contains(text, "```") {
		t.Fatalf("Markdown fence in Jira wiki text: %.200q", text)
	}
	if strings.Count(text, "{noformat}")%2 != 0 {
		t.Fatalf("{noformat} block is open: %.200q", text)
	}
}
