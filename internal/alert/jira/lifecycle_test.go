package jira

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedJira(t *testing.T) (*Jira, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"OPS-9"}`))
	}
	j := NewJira(map[string]interface{}{
		"url": rec.URL(), "user": "u", "apiToken": "secret",
		"projectKey": "OPS", "closeTransition": "",
	}, "dev", rec.Dependencies())
	if j == nil {
		t.Fatal("jira was not constructed")
	}
	return j, rec
}

// With the closing transition turned off, recovery is a comment on the
// same issue.
func TestJiraIncidentLifecycleFollowsOneIssue(t *testing.T) {
	j, rec := newRecordedJira(t)
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:secret"))
	want := map[string]string{
		"announce": "POST /rest/api/2/issue",
		"update":   "POST /rest/api/2/issue/OPS-9/comment",
		"resolve":  "POST /rest/api/2/issue/OPS-9/comment",
	}
	for _, tc := range providertest.Lifecycle() {
		rec.Reset()
		if err := j.SendIncident(context.Background(), tc.Message); err != nil {
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
		if req.Header.Get("Authorization") != auth {
			t.Fatalf("%s lacks basic auth", tc.Name)
		}
		body := req.JSON(t)
		text, _ := body["body"].(string)
		if tc.Name == "announce" {
			fields := body["fields"].(map[string]any)
			if fields["summary"] != tc.Message.Short {
				t.Fatalf("summary = %v", fields["summary"])
			}
			providertest.AssertOneLeadingEmoji(t, fields["summary"].(string))
			text, _ = fields["description"].(string)
			if !strings.Contains(text, "{noformat}") {
				t.Fatalf("output is not a code block: %q", text)
			}
		}
		providertest.AssertOneLeadingEmoji(t, text)
		if !strings.Contains(text, tc.Message.Note) {
			t.Fatalf("%s text = %q", tc.Name, text)
		}
	}
	// No close setting: the resolve only commented, the issue is still
	// open, so it stays mapped for a recurrence.
	if len(j.SnapshotThreads()) != 1 {
		t.Fatal("an unclosed issue must stay tracked")
	}
}

func TestJiraNoticeOpensNoIssue(t *testing.T) {
	j, rec := newRecordedJira(t)
	if err := j.SendMessage(context.Background(), "started"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Requests()); n != 0 {
		t.Fatalf("notice sent %d requests", n)
	}
}

func TestJiraCreateFailureIsReturned(t *testing.T) {
	j, rec := newRecordedJira(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := j.SendIncident(context.Background(), providertest.Announce())
	if err == nil {
		t.Fatal("forbidden create must fail")
	}
}
