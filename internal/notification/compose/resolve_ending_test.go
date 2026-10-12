package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func resolvedAfterHold(p incident.Incident) incident.Incident {
	p.State = incident.Resolved
	p.Opened = writerNow.Add(-7 * time.Minute)
	p.RecoveringSince = writerNow.Add(-3 * time.Minute)
	p.Resolved = writerNow
	return p
}

// The hold before a resolve is waiting, not failing: the message counts
// to the moment the last failure cleared.
func TestResolveDurationStopsWhenTheFailureCleared(t *testing.T) {
	p := resolvedAfterHold(podCrash(incident.Notify))

	_, got := resolveNote(caseFacts{p: p, now: writerNow})

	text := got[len(got)-1].text
	if !strings.Contains(text, "failing for four minutes") {
		t.Fatalf("want the failing time without the hold, got %q", text)
	}
}

func TestResolveSaysDeletedWhenTheSubjectWasDeleted(t *testing.T) {
	tests := []struct {
		name      string
		namespace bool
		want      string
	}{
		{"object", false, "Pod api-1 in default was deleted."},
		{"namespace", true,
			"Pod api-1 in default was deleted with its namespace."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := resolvedAfterHold(podCrash(incident.Notify))
			p.Removed = []inventory.EntityID{p.Root}
			p.NamespaceRemoved = tt.namespace
			fix := inventory.Change{Entity: p.Root, Actor: "bob",
				Revision: "9", At: writerNow}

			_, got := resolveNote(caseFacts{p: p, now: writerNow,
				fix: &fix})

			if lead := got[0].text; lead != tt.want {
				t.Fatalf("lead %q, want %q", lead, tt.want)
			}
			if text := got[1].text; strings.Contains(text, "bob") ||
				strings.Contains(text, "rolled") {
				t.Fatalf("a deletion has no fixer, got %q", text)
			}
		})
	}
}

// A resolve has no members left to lead it: it names the subject the
// announcement named.
func TestResolveNamesTheAnnouncedSubject(t *testing.T) {
	p := resolvedAfterHold(podCrash(incident.Notify))
	p.Members = nil
	p.Root = inventory.CoreID(kube.KindAccount, "shop", "default")
	p.Subject = inventory.CoreID(kube.KindDeployment, "shop", "storm")

	_, got := resolveNote(caseFacts{p: p, now: writerNow})

	if !strings.Contains(got[0].text, "storm in shop is healthy again") {
		t.Fatalf("want the announced subject, got %q", got[0].text)
	}
}

func TestResolveFallsBackToTheRootWithoutASubject(t *testing.T) {
	p := resolvedAfterHold(podCrash(incident.Notify))
	p.Members = nil

	_, got := resolveNote(caseFacts{p: p, now: writerNow})

	if !strings.Contains(got[0].text, "Pod api-1 in default") {
		t.Fatalf("want the root, got %q", got[0].text)
	}
}
