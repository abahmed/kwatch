package dashboard

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A release that went wrong names the issue it opened even when that
// issue is resolved or the release watch did not link it; one with no
// issue on its workload in the window names none, and the summary
// says the windows it used.
func TestLinkReleaseIssuesFindsResolvedAndUnlinkedIssues(t *testing.T) {
	at := testEpoch.Add(-2 * time.Hour)
	onAPI := func(id string, opened time.Time) incident.Incident {
		p := incident.Incident{ID: id, Tier: incident.Notify,
			State: incident.Resolved, Opened: opened,
			Resolved: opened.Add(5 * time.Minute),
			Root:     inventory.CoreID(kube.KindDeployment, "shop", "api")}
		return p
	}
	items := []incident.Incident{
		onAPI("late", at.Add(3*time.Hour)),
		onAPI("early", at.Add(-time.Hour)),
		onAPI("hit", at.Add(10*time.Minute)),
		onAPI("linked", at.Add(20*time.Minute)),
	}
	sum := &ReleaseSummary{Wrong: []WrongRelease{
		{Kind: "deployment", Namespace: "shop", Name: "api", At: at},
		{Kind: "deployment", Namespace: "shop", Name: "api", At: at,
			Incident: "linked"},
		{Kind: "deployment", Namespace: "shop", Name: "web", At: at},
	}}
	linkReleaseIssues(sum, items, nil)
	got := func(i int) string {
		if sum.Wrong[i].Issue == nil {
			return ""
		}
		return sum.Wrong[i].Issue.ID + "/" + sum.Wrong[i].Issue.State
	}
	if got(0) != "hit/resolved" || got(1) != "linked/resolved" ||
		got(2) != "" {
		t.Errorf("issues = %q %q %q", got(0), got(1), got(2))
	}
	if sum.WindowHours != 24 || sum.MatchMinutes != 30 {
		t.Errorf("windows = %d h, %d min", sum.WindowHours,
			sum.MatchMinutes)
	}
}
