package gitea

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedGitea(t *testing.T) (*Gitea, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":5}`))
	}
	g := NewGitea(map[string]interface{}{
		"token": "secret", "owner": "o", "repo": "r", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	if g == nil {
		t.Fatal("gitea was not constructed")
	}
	return g, rec
}

func TestGiteaIncidentLifecycleFollowsOneIssue(t *testing.T) {
	g, rec := newRecordedGitea(t)
	want := map[string][]string{
		"announce": {"POST /repos/o/r/issues"},
		"update":   {"POST /repos/o/r/issues/5/comments"},
		"resolve": {
			"POST /repos/o/r/issues/5/comments", "PATCH /repos/o/r/issues/5",
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
			if req.Header.Get("Authorization") != "token secret" {
				t.Fatalf("%s lacks the token", tc.Name)
			}
		}
		first := requests[0].JSON(t)
		if tc.Name == "announce" {
			if first["title"] != tc.Message.Short {
				t.Fatalf("title = %v", first["title"])
			}
			providertest.AssertOneLeadingEmoji(t, first["title"].(string))
		}
		body, _ := first["body"].(string)
		providertest.AssertOneLeadingEmoji(t, body)
		if !strings.Contains(body, tc.Message.Note) {
			t.Fatalf("%s body = %q", tc.Name, body)
		}
	}
	if last := rec.Last(t).JSON(t); last["state"] != "closed" {
		t.Fatalf("resolve did not close: %v", last)
	}
	if len(g.SnapshotThreads()) != 0 {
		t.Fatal("closed issue is still tracked")
	}
}

func TestGiteaNoticeOpensNoIssue(t *testing.T) {
	g, rec := newRecordedGitea(t)
	if err := g.SendMessage(context.Background(), "started"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Requests()); n != 0 {
		t.Fatalf("notice sent %d requests", n)
	}
}

func TestGiteaCreateFailureIsReturned(t *testing.T) {
	g, rec := newRecordedGitea(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	err := g.SendIncident(context.Background(), providertest.Announce())
	if err == nil {
		t.Fatal("unauthorized create must fail")
	}
}
