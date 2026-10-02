package jira

import (
	"context"
	"net/http"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A 2xx create whose body has no readable reference must not lead to a
// second create on the following update or resolve.
func TestJiraUnreadableCreateOpensOneIssue(t *testing.T) {
	j, rec := newRecordedJira(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`<html>created</html>`))
	}
	for _, tc := range providertest.Lifecycle() {
		if err := j.SendIncident(context.Background(), tc.Message); err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
	}
	requests := rec.Requests()
	if len(requests) != 1 || requests[0].Method != http.MethodPost {
		t.Fatalf("sent %d requests, want one create", len(requests))
	}
	if len(j.SnapshotThreads()) != 0 {
		t.Fatal("untracked marker was persisted as an issue")
	}
}
