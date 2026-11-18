package dashboard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// leftoverGroup is a leftover issue titled when it held 4 objects and
// now holding 2.
func leftoverGroup() incident.Incident {
	root := objID("service", "ops", "hook-svc")
	p := holderIncident(root, detection.ModeStuckDeleting)
	p.Cause = nil
	for _, o := range []string{"secret/hook-cert",
		"validatingwebhookconfiguration/hook"} {
		kind, name, _ := strings.Cut(o, "/")
		e := objID(kind, "ops", name)
		p.Members[detection.Key{Entity: e,
			Reason: reasons.WebhookNoEndpoints}] = detection.Finding{
			Entity: e, Reason: reasons.WebhookNoEndpoints, Leftover: root}
	}
	p.Problem = "Leftovers of a removed or stopped controller (4 " +
		"objects): service ops/hook-svc, which its webhooks call, has " +
		"no ready pods."
	p.ProblemFull = p.Problem
	return p
}

// The count in the title, "What's wrong?" and the list come from one
// set: the live members, not the set the title was written for.
func TestGroupCountsAgreeAcrossTitleSummaryAndList(t *testing.T) {
	p := leftoverGroup()
	d := toDetail(p)

	assert.Contains(t, d.Title, "(2 objects)")
	assert.Contains(t, d.FullTitle, "(2 objects)")
	assert.Contains(t, d.Summary.What, ": 2 objects it left behind")
	assert.Len(t, d.Leftovers, 2)
	assert.Equal(t, 2, toListItem(p).LeftoverObjects)
	assert.Contains(t, toListItem(p).Title, "(2 objects)")
}

// A title that opens with its count and names the few it was written
// for is given the live count, and the stale names are left out.
func TestGroupTitleTakesTheLiveCount(t *testing.T) {
	p := holderIncident(objID("finalizer", "shop", "example.com/cleanup"),
		detection.ModeStuckDeleting, objID("secret", "shop", "s"),
		objID("role", "shop", "r"), objID("rolebinding", "shop", "rb"),
		objID("serviceaccount", "shop", "sa"))
	p.Cause = &rootcause.CauseRecord{Root: p.Root}
	p.Problem = "Two objects (secret s and service account sa) in shop " +
		"have been stuck deleting since Dec 4: their finalizer " +
		"example.com/cleanup is never removed."
	d := toDetail(p)

	assert.Equal(t, "Four objects in shop have been stuck deleting since "+
		"Dec 4: their finalizer example.com/cleanup is never removed.",
		d.Title)
	assert.Contains(t, d.Summary.What, "Each of 4 objects in shop")
}

func TestLiveTitleLeavesOtherTitlesAlone(t *testing.T) {
	for _, c := range []struct {
		title string
		n     int
	}{
		{"api in shop keeps crashing.", 3},
		{"Leftovers of a removed controller (3 objects): x is gone.", 3},
		{"Two objects (a and b) in shop are stuck.", 2},
		{"Two objects (a and b) in shop are stuck.", 1},
		{"One object in shop is stuck.", 3},
	} {
		assert.Equal(t, c.title, liveTitle(c.title, c.n), c.title)
	}
	assert.Equal(t, "Leftovers (12 objects): x.",
		liveTitle("Leftovers (3 objects): x.", 12))
	assert.Equal(t, "11 objects in shop are stuck.",
		liveTitle("Two objects (a and b) in shop are stuck.", 11))
}
