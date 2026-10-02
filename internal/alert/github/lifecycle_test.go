package github

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedGithub(t *testing.T) (*Github, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42}`))
	}
	g := NewGithub(map[string]interface{}{
		"token": "secret", "owner": "kwatch", "repo": "kwatch",
		"url": rec.URL(),
	}, "dev", rec.Dependencies())
	if g == nil {
		t.Fatal("github was not constructed")
	}
	return g, rec
}

func TestGithubIncidentLifecycleFollowsOneIssue(t *testing.T) {
	g, rec := newRecordedGithub(t)
	issues := "/repos/kwatch/kwatch/issues"
	want := map[string][]string{
		"announce": {"POST " + issues},
		"update":   {"POST " + issues + "/42/comments"},
		"resolve": {
			"POST " + issues + "/42/comments", "PATCH " + issues + "/42",
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
			if req.Header.Get("Authorization") != "Bearer secret" {
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

func TestGithubRestoredIssueReceivesComments(t *testing.T) {
	g, rec := newRecordedGithub(t)
	g.RestoreThreads(map[string]string{
		providertest.Announce().ThreadKey(): "7",
	})
	err := g.SendIncident(context.Background(), providertest.Update())
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.Last(t).Path; got != "/repos/kwatch/kwatch/issues/7/comments" {
		t.Fatalf("comment went to %s", got)
	}
}

func TestGithubNoticeOpensNoIssue(t *testing.T) {
	g, rec := newRecordedGithub(t)
	if err := g.SendMessage(context.Background(), "started"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Requests()); n != 0 {
		t.Fatalf("notice sent %d requests", n)
	}
}

func TestGithubCreateFailureIsReturned(t *testing.T) {
	g, rec := newRecordedGithub(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	err := g.SendIncident(context.Background(), providertest.Announce())
	if err == nil {
		t.Fatal("unauthorized create must fail")
	}
	if len(g.SnapshotThreads()) != 0 {
		t.Fatal("failed create must not be tracked")
	}
}
