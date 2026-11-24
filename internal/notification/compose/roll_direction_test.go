package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The fix's revision against the blamed change's says which way the
// workload went: a higher revision is a release forward, never a
// rollback, even when it equals the revision a rollback would return
// to; a lower one is a rollback.
func TestResolveSaysWhichWayTheRevisionMoved(t *testing.T) {
	deploy := inventory.CoreID("deployment", "shop", "storage")
	for name, tc := range map[string]struct {
		blamed, rollback, fixed string
		want                    string
	}{
		"forward past the blamed revision": {"1057", "1058", "1058",
			"argocd rolled out revision 1058"},
		"back below the blamed revision": {"1058", "1057", "1057",
			"argocd rolled back to revision 1057"},
		"numbers unknown, rollback target": {"", "abc", "abc",
			"argocd rolled back to revision abc"},
	} {
		t.Run(name, func(t *testing.T) {
			p := podCrash(incident.Notify)
			p.State, p.Resolved = incident.Resolved, writerNow
			p.Cause = &rootcause.CauseRecord{Root: deploy, Score: 0.95,
				Rule: "rollout", RollbackRevision: tc.rollback,
				Change: &inventory.Change{Entity: deploy,
					Revision: tc.blamed}}
			d := incident.Decision{Action: incident.Resolve, Incident: p}
			fix := inventory.Change{Entity: deploy, Actor: "argocd",
				Revision: tc.fixed, At: writerNow}

			msg := Writer{}.WriteResolvedBy(d, writerNow, fix)

			assert.Contains(t, msg.Note, tc.want)
		})
	}
}

// A fix the history calls a rollback is a rollback, whatever its
// revision number: a rollout undo gives the template it brings back
// the next number. Only a fix that set the blamed fields to new values
// after the revert is a new revision.
func TestEndedWordsFollowTheContent(t *testing.T) {
	deploy := inventory.CoreID("deployment", "shop", "storage")
	blamed := &inventory.Change{Revision: "1058",
		Fields: []inventory.FieldChange{{Path: "image",
			Before: "api:1", After: "api:2"}}}
	f := caseFacts{p: incident.Incident{Fix: incident.FixRollback,
		Cause: &rootcause.CauseRecord{Root: deploy, Change: blamed}},
		fix: &inventory.Change{Revision: "1059"}}
	assert.Equal(t, "; a rollback fixed it", endedWords(f, deploy))
	f.fix.Fields = []inventory.FieldChange{{Path: "image",
		Before: "api:1", After: "api:3"}}
	assert.Equal(t, "; a new revision fixed it", endedWords(f, deploy))
}

// Revision 3 is blamed; `kubectl rollout undo` brings back the template
// of revision 2 as revision 4. The content says it went back, so the
// resolve says rolled back, not "rolled out revision 4".
func TestRolloutUndoToAHigherNumberIsARollback(t *testing.T) {
	deploy := inventory.CoreID("deployment", "shop", "storage")
	blamed := &inventory.Change{Entity: deploy, Revision: "3",
		Fields: []inventory.FieldChange{{Path: "image",
			Before: "api:2", After: "api:3"}}}
	undo := inventory.Change{Entity: deploy, Actor: "alice",
		Revision: "4", At: writerNow,
		Fields: []inventory.FieldChange{{Path: "image",
			Before: "api:3", After: "api:2"}}}
	for name, fix := range map[string]incident.Fix{
		"history says reverted": incident.FixRollback,
		"fields say reverted":   incident.FixChange,
	} {
		p := podCrash(incident.Notify)
		p.State, p.Resolved, p.Fix = incident.Resolved, writerNow, fix
		p.Cause = &rootcause.CauseRecord{Root: deploy, Score: 0.95,
			Rule: "rollout", RollbackRevision: "2", Change: blamed}
		d := incident.Decision{Action: incident.Resolve, Incident: p}

		msg := Writer{}.WriteResolvedBy(d, writerNow, undo)

		assert.Contains(t, msg.Note, "alice rolled back to the earlier "+
			"version (now revision 4)", name)
		assert.NotContains(t, msg.Note, "rolled out revision", name)
	}
}
