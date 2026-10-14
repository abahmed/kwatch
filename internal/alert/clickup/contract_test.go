package clickup

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const (
	testToken = "pk_clickup_secret"
	// createdID is the task id the recorder answers a create with.
	createdID = "t1"
)

var taskRef = regexp.MustCompile(`^/task/([^/]+)`)

// TestContract pins the exact ClickUp requests: the task create, the
// comment and the closing status change.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + createdID + `"}`))
	}
	c := NewClickup(map[string]interface{}{
		"token": testToken, "listId": "abc123", "priority": 2,
		"closeStatus": "complete",
	}, "dev", rec.Dependencies())
	c.api = rec.URL()
	c.url = rec.URL() + "/list/abc123/task"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{testToken},
		KeyOf:   taskKey,
		Check:   checkTaskShape,
	})
}

// taskKey is the task a request addresses; a create addresses the one
// the recorder numbered, so every lifecycle step lands on one task.
func taskKey(t *testing.T, r providertest.Request) string {
	if m := taskRef.FindStringSubmatch(r.Path); m != nil {
		return m[1]
	}
	if r.Method == http.MethodPost {
		return createdID
	}
	t.Fatalf("request addresses no task: %s %s", r.Method, r.Path)
	return ""
}

func checkTaskShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	for _, r := range reqs {
		if r.Header.Get("Authorization") != testToken {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
	}
	switch name {
	case "digest":
		if len(reqs) != 0 {
			t.Fatal("a digest must not open a task")
		}
	case "notify-update":
		if len(reqs) != 1 {
			t.Fatalf("update sent %d requests", len(reqs))
		}
		checkComment(t, reqs[0])
	case "resolve":
		if len(reqs) != 2 {
			t.Fatalf("resolve sent %d requests", len(reqs))
		}
		checkComment(t, reqs[0])
		if reqs[1].Method != "PUT" || reqs[1].Path != "/task/t1" ||
			reqs[1].JSON(t)["status"] != "complete" {
			t.Fatalf("resolve must set the close status: %+v", reqs[1])
		}
	default:
		checkCreate(t, reqs)
	}
}

func checkComment(t *testing.T, r providertest.Request) {
	text, _ := r.JSON(t)["comment_text"].(string)
	if r.Method != "POST" || r.Path != "/task/t1/comment" || text == "" {
		t.Fatalf("comment = %s %s %q", r.Method, r.Path, text)
	}
	providertest.AssertFencesBalanced(t, text)
}

func checkCreate(t *testing.T, reqs []providertest.Request) {
	if len(reqs) != 1 {
		t.Fatalf("create sent %d requests", len(reqs))
	}
	r := reqs[0]
	p := r.JSON(t)
	name, _ := p["name"].(string)
	desc, _ := p["markdown_description"].(string)
	if r.Method != "POST" || r.Path != "/list/abc123/task" ||
		name == "" || desc == "" || p["priority"] != 2.0 {
		t.Fatalf("create = %s %s %v", r.Method, r.Path, p)
	}
	if _, plain := p["description"]; plain {
		t.Fatal("markdown text must go in markdown_description")
	}
	if n := utf8.RuneCountInString(name); n > titleLimit {
		t.Fatalf("name is %d runes, limit %d", n, titleLimit)
	}
	providertest.AssertOneLeadingEmoji(t, name)
	if strings.Contains(desc, "Ref: kwatchkey") {
		t.Fatal("ClickUp has no search, so no marker is expected")
	}
	providertest.AssertFencesBalanced(t, desc)
}
