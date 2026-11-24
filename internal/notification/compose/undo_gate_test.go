package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// undoStep reports a step that undoes a release: a rollout undo, a Helm
// rollback or a revert in Git.
func undoStep(s notification.Step) bool {
	return strings.Contains(s.Command, "rollout undo") ||
		strings.HasPrefix(s.Command, "helm rollback") ||
		strings.HasPrefix(s.Text, "Revert the change in Git")
}

func assertNoUndo(t *testing.T, steps []notification.Step) {
	t.Helper()
	for _, s := range steps {
		assert.False(t, undoStep(s), "undo step offered: %+v", s)
		assert.False(t, s.Mutating, "mutating step offered: %+v", s)
	}
}

// possibleRollout is the bad rollout blamed only at "possible": its
// score is under Likely, or a near rival shares the failures.
func possibleRollout(rival bool, app string) incident.Incident {
	p := badRollout()
	p.Tier = incident.Notify
	p.Cause.PreviousServed = true
	p.Cause.Change.App = app
	if rival {
		other := *p.Cause
		other.Change, other.Root = nil, paymentsCt
		p.Cause.Rival = &other
		return p
	}
	p.Cause.Score = 0.4
	return p
}

// A release blamed only at "possible" gets no undo, in the message's
// steps or in the steps the dashboard stores from them: only the
// read-only checks, and the one that would confirm the release, also
// for a release tied with a rival, which the message does not name.
func TestPossibleReleaseOffersNoUndo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rival bool
		app   string
	}{
		{"low score", false, ""},
		{"near rival", true, ""},
		{"gitops", false, "argocd/shop"},
		{"helm", true, "helm/payments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := possibleRollout(tc.rival, tc.app)
			require.Equal(t, rootcause.LevelPossible,
				rootcause.CertaintyOf(p.Cause))
			msg := Writer{}.Write(announce(p), at(5, 0))
			assertNoUndo(t, msg.Steps)
			assert.NotContains(t, msg.Note, "rollout undo")
			assert.NotContains(t, msg.Note, "helm rollback")
			require.NotEmpty(t, msg.Steps)
			assert.Equal(t, "kubectl rollout status "+
				"deployment/payments -n shop", msg.Steps[0].Command)
			assert.Contains(t, stepTexts(msg.Steps),
				"confirms the release is the cause")
		})
	}
}

// A failed Helm release whose cause is not at least likely gets the
// read-only steps, not the rollback.
func TestHelmRollbackNeedsALikelyCause(t *testing.T) {
	release := kube.HelmReleaseID("shop", "payments")
	failed := detection.Finding{Entity: release,
		Reason: reasons.HelmReleaseFailed, Evidence: []detection.Evidence{
			{Label: detection.EvidenceHelmGoodRevision, Value: "11"}}}
	members := []detection.Finding{failed}
	assertNoUndo(t, helmSteps(release, nil, members))
	assertNoUndo(t, helmSteps(release,
		&rootcause.CauseRecord{Root: release, Score: 0.4}, members))
	steps := helmSteps(release,
		&rootcause.CauseRecord{Root: release, Score: 0.6}, members)
	require.Len(t, steps, 3)
	assert.Equal(t, "helm rollback payments 11 -n shop", steps[2].Command)
}

func stepTexts(steps []notification.Step) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString(s.Text + " " + s.Command + "\n")
	}
	return b.String()
}

// A likely release whose previous revision kwatch never saw serve gets
// no rollback: the steps and the message say why, and keep the
// read-only checks. With the record, the rollback is offered.
func TestRollbackNeedsARecordOfThePreviousRevisionServing(t *testing.T) {
	for _, app := range []string{"", "argocd/shop", "helm/payments"} {
		t.Run("app="+app, func(t *testing.T) {
			p := badRollout()
			p.Cause.Change.App = app
			p.Cause.PreviousServed = false
			msg := Writer{}.Write(announce(p), at(5, 0))
			assertNoUndo(t, msg.Steps)
			assert.Contains(t, msg.Note, "kwatch has no record of "+
				"revision 13 running healthily, so it does not suggest "+
				"rolling back to it.")
			assert.NotContains(t, msg.Note, "Rolling back")
			assert.NotContains(t, msg.Note, "Revert the change in Git")
			require.NotEmpty(t, msg.Steps)
			assert.Equal(t, "kubectl rollout status "+
				"deployment/payments -n shop", msg.Steps[0].Command)

			p.Cause.PreviousServed = true
			p.Cause.HelmRollbackRevision = "12"
			served := Writer{}.Write(announce(p), at(5, 0))
			assert.True(t, hasUndo(served.Steps), "undo with the record")
			assert.NotContains(t, served.Note, withheldLead)
		})
	}
}

func hasUndo(steps []notification.Step) bool {
	for _, s := range steps {
		if undoStep(s) {
			return true
		}
	}
	return false
}
