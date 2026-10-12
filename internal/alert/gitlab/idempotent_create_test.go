package gitlab

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A create whose answer was lost may have succeeded. The announce looks the
// incident's marker up first and adopts the issue it finds instead of
// opening a second one.
func TestGitlabAnnounceAdoptsExistingIssue(t *testing.T) {
	g, rec := newRecordedGitlab(t)
	marker := issues.Marker(providertest.Announce().ThreadKey())
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`[{"iid":9,"description":"x ` + marker + `"}]`))
	}

	err := g.SendIncident(context.Background(), providertest.Announce())

	if err != nil {
		t.Fatal(err)
	}
	requests := rec.Requests()
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Fatalf("want one search and no create, got %d requests",
			len(requests))
	}
	target := requests[0].Path + "?" + requests[0].Query
	if !strings.Contains(target, "search="+marker) {
		t.Fatalf("search %s lacks the marker", target)
	}
	if !g.HasThread(providertest.Announce().ThreadKey()) {
		t.Fatal("the found issue was not tracked")
	}
}

func TestGitlabAnnounceCreatesWithMarkerWhenNoneFound(t *testing.T) {
	g, rec := newRecordedGitlab(t)
	marker := issues.Marker(providertest.Announce().ThreadKey())

	if err := g.SendIncident(
		context.Background(), providertest.Announce()); err != nil {
		t.Fatal(err)
	}

	requests := rec.Requests()
	if len(requests) != 2 || requests[1].Method != http.MethodPost {
		t.Fatalf("want search then create, got %d requests", len(requests))
	}
	if !strings.Contains(string(requests[1].Body), "Ref: "+marker) {
		t.Fatalf("create body lacks the marker: %s", requests[1].Body)
	}
}
