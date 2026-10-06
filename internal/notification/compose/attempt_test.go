package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func attemptFacts() caseFacts {
	api := inventory.CoreID(kube.KindDeployment, "shop", "payments")
	change := inventory.Change{Entity: api, Actor: "alice", Revision: "15",
		At: at(30, 0)}
	return caseFacts{now: at(40, 0), fix: &change, p: incident.Incident{
		Root: api, Attempt: &change, Resolved: at(48, 0)}}
}

func TestAttemptSentencesSayWhatStartedAndWhatFailed(t *testing.T) {
	f := attemptFacts()
	if got := attemptSentences(f)[0].text; got !=
		"Rollout 15 of payments in shop started at 14:30 (alice); watching." {
		t.Errorf("started: %q", got)
	}
	if got := stillFailingSentences(f)[0].text; got !=
		"Still failing 10 minutes after rollout 15." {
		t.Errorf("late: %q", got)
	}
	want := "Fixed by rollout 15 (alice) after 18 minutes"
	if got := fixedByPhrase(f); got != want {
		t.Errorf("resolve: %q, want %q", got, want)
	}
	f.p.Attempt = nil
	if fixedByPhrase(f) != "" {
		t.Error("without an attempt the usual wording stays")
	}
}
