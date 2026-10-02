package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedGitlab(t *testing.T) (*Gitlab, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":3}`))
	}
	g := NewGitlab(map[string]interface{}{
		"token": "secret", "projectId": "7", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	if g == nil {
		t.Fatal("gitlab was not constructed")
	}
	return g, rec
}

func TestGitlabIncidentLifecycleFollowsOneIssue(t *testing.T) {
	g, rec := newRecordedGitlab(t)
	want := map[string][]string{
		"announce": {"POST /projects/7/issues"},
		"update":   {"POST /projects/7/issues/3/notes"},
		"resolve": {
			"POST /projects/7/issues/3/notes", "PUT /projects/7/issues/3",
		},
	}
	for _, tc := range providertest.Lifecycle() {
		rec.Reset()
		if err := g.SendIncident(context.Background(), tc.Message); err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		requests := rec.Requests()
		if len(requests) != len(want[tc.Name]) {
			t.Fatalf("%s sent %d requests", tc.Name, len(requests))
		}
		for i, req := range requests {
			if got := req.Method + " " + req.Path; got != want[tc.Name][i] {
				t.Fatalf("%s request %d = %s", tc.Name, i, got)
			}
			if req.Header.Get("PRIVATE-TOKEN") != "secret" {
				t.Fatalf("%s lacks the token", tc.Name)
			}
		}
		first := requests[0].JSON(t)
		field := "body"
		if tc.Name == "announce" {
			field = "description"
			if first["title"] != tc.Message.Short {
				t.Fatalf("title = %v", first["title"])
			}
			providertest.AssertOneLeadingEmoji(t, first["title"].(string))
		}
		body, _ := first[field].(string)
		providertest.AssertOneLeadingEmoji(t, body)
		if !strings.Contains(body, tc.Message.Note) {
			t.Fatalf("%s %s = %q", tc.Name, field, body)
		}
	}
	if last := rec.Last(t).JSON(t); last["state_event"] != "close" {
		t.Fatalf("resolve did not close: %v", last)
	}
}

func TestGitlabClosesIssueForGroupProject(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			requests = append(requests, r.Method+" "+r.URL.EscapedPath())
			mu.Unlock()
			if r.Method == http.MethodPost &&
				r.URL.EscapedPath() == "/projects/grp%2Fapp/issues" {
				_, _ = w.Write([]byte(`{"iid":3}`))
			}
		}))
	defer s.Close()
	c := NewGitlab(map[string]interface{}{
		"token": "t", "projectId": "grp/app", "url": s.URL,
	}, testAppConfig(), testDeps)
	ctx := context.Background()
	assert.NoError(t, c.SendIncident(ctx, providertest.Announce()))
	assert.NoError(t, c.SendIncident(ctx, providertest.Resolve()))
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"POST /projects/grp%2Fapp/issues",
		"POST /projects/grp%2Fapp/issues/3/notes",
		"PUT /projects/grp%2Fapp/issues/3",
	}, requests)
}

func TestGitlabNoticeOpensNoIssue(t *testing.T) {
	g, rec := newRecordedGitlab(t)
	if err := g.SendMessage(context.Background(), "started"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Requests()); n != 0 {
		t.Fatalf("notice sent %d requests", n)
	}
}

func TestGitlabCreateFailureIsReturned(t *testing.T) {
	g, rec := newRecordedGitlab(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	err := g.SendIncident(context.Background(), providertest.Announce())
	if err == nil {
		t.Fatal("unauthorized create must fail")
	}
}
