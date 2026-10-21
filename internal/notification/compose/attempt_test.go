package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
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
		"Rollout 15 of payments in shop started at 14:30 UTC (alice); "+
			"watching." {
		t.Errorf("started: %q", got)
	}
	if got := stillFailingSentences(f)[0].text; got !=
		"payments in shop is still failing 10 minutes after rollout 15." {
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

// The resolve measures a fix against the moment the failures cleared, not
// against the end of the resolve hold that followed.
func TestFixedByPhraseMeasuresToTheRecovery(t *testing.T) {
	f := attemptFacts()
	f.p.RecoveringSince = at(40, 0)
	f.p.Opened = at(0, 0)
	f.p.Resolved = at(48, 0)
	want := "Fixed by rollout 15 (alice) after 10 minutes"
	if got := fixedByPhrase(f); got != want {
		t.Errorf("fixed by = %q, want %q", got, want)
	}
}

// A message may be read by a provider that shows only the Short: it must
// say what the update is about.
func TestAttemptShortNamesTheWorkload(t *testing.T) {
	f := attemptFacts()
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	other := inventory.Change{Entity: cm, At: at(30, 0)}
	f.p.Attempt = &other
	for name, sentences := range map[string][]sentence{
		"started": attemptSentences(f), "still failing": stillFailingSentences(f),
	} {
		var msg notification.Message
		fill(&msg, "🟠", sentences)
		if !strings.Contains(msg.Short, "payments") {
			t.Errorf("%s: Short %q does not name the workload", name,
				msg.Short)
		}
	}
}

func TestClockNamesItsZone(t *testing.T) {
	if got := clock(at(30, 0)); got != "14:30 UTC" {
		t.Errorf("clock = %q", got)
	}
}
