package clickup

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedClickup(t *testing.T) (*Clickup, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"id":"t1"}`))
	}
	c := NewClickup(map[string]interface{}{
		"token": "secret", "listId": "abc123", "priority": 2,
	}, "dev", rec.Dependencies())
	if c == nil {
		t.Fatal("clickup was not constructed")
	}
	c.api = rec.URL()
	c.url = rec.URL() + "/list/abc123/task"
	return c, rec
}

// ClickUp status names are defined per list, so recovery is a comment on
// the same task.
func TestClickupIncidentLifecycleFollowsOneTask(t *testing.T) {
	c, rec := newRecordedClickup(t)
	want := map[string]string{
		"announce": "POST /list/abc123/task",
		"update":   "POST /task/t1/comment",
		"resolve":  "POST /task/t1/comment",
	}
	for _, tc := range providertest.Lifecycle() {
		rec.Reset()
		if err := c.SendIncident(context.Background(), tc.Message); err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		requests := rec.Requests()
		if len(requests) != 1 {
			t.Fatalf("%s sent %d requests", tc.Name, len(requests))
		}
		req := requests[0]
		if got := req.Method + " " + req.Path; got != want[tc.Name] {
			t.Fatalf("%s request = %s", tc.Name, got)
		}
		if req.Header.Get("Authorization") != "secret" {
			t.Fatalf("%s lacks the token", tc.Name)
		}
		body := req.JSON(t)
		text, _ := body["comment_text"].(string)
		if tc.Name == "announce" {
			if body["name"] != tc.Message.Short || body["priority"] != 2.0 {
				t.Fatalf("task = %v", body)
			}
			providertest.AssertOneLeadingEmoji(t, body["name"].(string))
			text, _ = body["description"].(string)
		}
		providertest.AssertOneLeadingEmoji(t, text)
		if !strings.Contains(text, tc.Message.Note) {
			t.Fatalf("%s text = %q", tc.Name, text)
		}
	}
	if len(c.SnapshotThreads()) != 0 {
		t.Fatal("resolved task is still tracked")
	}
}

func TestClickupTracksTaskIDByAlertKey(t *testing.T) {
	c, _ := newRecordedClickup(t)
	msg := providertest.Announce()
	if err := c.SendIncident(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if got := c.SnapshotThreads()[msg.ThreadKey()]; got != "t1" {
		t.Fatalf("tracked task = %q", got)
	}
}

func TestClickupNoticeOpensNoTask(t *testing.T) {
	c, rec := newRecordedClickup(t)
	if err := c.SendMessage(context.Background(), "started"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Requests()); n != 0 {
		t.Fatalf("notice sent %d requests", n)
	}
}

func TestClickupCreateFailureIsReturned(t *testing.T) {
	c, rec := newRecordedClickup(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	if err == nil {
		t.Fatal("unauthorized create must fail")
	}
}
