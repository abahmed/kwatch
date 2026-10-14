package gitlab

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const (
	testToken = "gitlab-secret-token"
	// createdID is the issue number the recorder answers a create with.
	createdID = "3"
)

var issuePath = regexp.MustCompile(`/issues/(\d+)`)

// TestContract pins the exact GitLab issue requests: the idempotency
// search, the create, the comment and the close.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && !issuePath.MatchString(r.Path):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"iid":` + createdID + `}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}
	g := NewGitlab(map[string]interface{}{
		"token": testToken, "projectId": "group/project",
		"url": rec.URL(),
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: g.SendIncident, Rec: rec,
		Headers: []string{"PRIVATE-TOKEN"},
		Secrets: []string{testToken},
		KeyOf:   issueKey,
		Check:   checkIssueShape,
	})
}

// issueKey is the issue a request addresses; a create addresses the
// issue the recorder numbered, so open, update and resolve must all
// land on one issue.
func issueKey(t *testing.T, r providertest.Request) string {
	if m := issuePath.FindStringSubmatch(r.Path); m != nil {
		return m[1]
	}
	if r.Method == http.MethodPost {
		return createdID
	}
	t.Fatalf("request addresses no issue: %s %s", r.Method, r.Path)
	return ""
}

func checkIssueShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	for _, r := range reqs {
		if r.Header.Get("PRIVATE-TOKEN") != testToken {
			t.Fatalf("PRIVATE-TOKEN = %q", r.Header.Get("PRIVATE-TOKEN"))
		}
	}
	base := "/projects/group%2Fproject/issues"
	switch name {
	case "digest":
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an issue")
		}
	case "notify-update":
		if len(reqs) != 1 {
			t.Fatalf("update sent %d requests", len(reqs))
		}
		checkComment(t, reqs[0], base+"/"+createdID+"/notes")
	case "resolve":
		if len(reqs) != 2 {
			t.Fatalf("resolve sent %d requests", len(reqs))
		}
		checkComment(t, reqs[0], base+"/"+createdID+"/notes")
		if reqs[1].Method != "PUT" || reqs[1].RawPath != base+"/"+
			createdID || reqs[1].JSON(t)["state_event"] != "close" {
			t.Fatalf("resolve must close the issue: %+v", reqs[1])
		}
	default:
		checkCreate(t, reqs, base)
	}
}

func checkComment(t *testing.T, r providertest.Request, path string) {
	body, _ := r.JSON(t)["body"].(string)
	if r.Method != "POST" || r.RawPath != path || body == "" {
		t.Fatalf("comment = %s %s %q", r.Method, r.RawPath, body)
	}
	providertest.AssertFencesBalanced(t, body)
}

func checkCreate(t *testing.T, reqs []providertest.Request, base string) {
	if len(reqs) != 2 {
		t.Fatalf("create sent %d requests, want search and create",
			len(reqs))
	}
	search, create := reqs[0], reqs[1]
	if search.Method != "GET" || search.RawPath != base ||
		!strings.Contains(search.Query, "state=opened") ||
		!strings.Contains(search.Query, "kwatchkey") {
		t.Fatalf("search = %s %s?%s", search.Method, search.Path,
			search.Query)
	}
	p := create.JSON(t)
	title, _ := p["title"].(string)
	body, _ := p["description"].(string)
	if create.Method != "POST" || create.RawPath != base || title == "" {
		t.Fatalf("create = %s %s %q", create.Method, create.RawPath, title)
	}
	if n := utf8.RuneCountInString(title); n > titleLimit {
		t.Fatalf("title is %d runes, limit %d", n, titleLimit)
	}
	providertest.AssertOneLeadingEmoji(t, title)
	if !strings.Contains(body, "Ref: kwatchkey") {
		t.Fatal("body lacks the idempotency marker")
	}
	providertest.AssertFencesBalanced(t, body)
}
