package clickup

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A create whose answer was lost may have succeeded. The announce lists
// the open tasks first and adopts the one carrying the marker.
func TestClickupAnnounceAdoptsExistingTask(t *testing.T) {
	c, rec := newRecordedClickup(t)
	marker := issues.Marker(providertest.Announce().ThreadKey())
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"tasks":[` +
			`{"id":"other","markdown_description":"x"},` +
			`{"id":"t9","markdown_description":"Ref: ` + marker +
			`"}]}`))
	}

	if err := c.SendIncident(
		context.Background(), providertest.Announce()); err != nil {
		t.Fatal(err)
	}

	requests := rec.Requests()
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Fatalf("want one search and no create, got %d requests",
			len(requests))
	}
	if !c.HasThread(providertest.Announce().ThreadKey()) {
		t.Fatal("the found task was not tracked")
	}
	if got := c.SnapshotThreads(); got[providertest.Announce().
		ThreadKey()] != "t9" {
		t.Fatalf("tracked %v, want t9", got)
	}
}

func TestClickupFindPagesUntilLastPage(t *testing.T) {
	c, rec := newRecordedClickup(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if strings.Contains(r.Query, "page=0") {
			_, _ = w.Write([]byte(`{"tasks":[{"id":"a"}]}`))
			return
		}
		_, _ = w.Write([]byte(
			`{"tasks":[{"id":"b","description":"kwatchkeyx"}]}`))
	}
	id, err := c.Find(context.Background(), "kwatchkeyx")
	if err != nil || id != "b" {
		t.Fatalf("Find = %q, %v", id, err)
	}
}
