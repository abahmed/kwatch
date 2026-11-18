package dashboard

import (
	"strings"

	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// withheldLead starts the text that says why no rollback is offered for
// a release kwatch has no record of serving. It is the words the
// message uses for the same case.
const withheldLead = "kwatch has no record of "

// undoFor is the undo the page offers for r, on the same two rules the
// message follows: the release is the blamed change of the incident's
// cause at a certainty of at least likely (rootcause.CertaintyOf), and
// kwatch saw the revision a rollback returns to serve
// (cause.PreviousServed). Every undo changes the cluster, so below
// either rule no undo is offered: the zero Undo, or a text saying why
// when a known revision lacks the evidence. An undo never goes without
// a revision. The second result is the revision the undo returns to,
// for release.Record.Revision.
func undoFor(r release.Record, cause *rootcause.CauseRecord) (
	release.Undo, string,
) {
	if !blames(cause, r) || !undoSure(cause) {
		return release.Undo{}, ""
	}
	revision := cause.RollbackRevision
	managed := managedBy(r.App)
	if !managed && revision == "" {
		return release.Undo{}, ""
	}
	if !cause.PreviousServed {
		return release.Undo{Text: withheldText(revision)}, ""
	}
	return sureUndo(r, cause, managed)
}

// undoSure reports a cause stated at likely or high, as
// rootcause.CertaintyOf words it: the message's own predicate.
func undoSure(cause *rootcause.CauseRecord) bool {
	level := rootcause.CertaintyOf(cause)
	return level == rootcause.LevelHigh || level == rootcause.LevelLikely
}

// sureUndo is the undo of a cause that passed both rules.
func sureUndo(
	r release.Record, cause *rootcause.CauseRecord, managed bool,
) (release.Undo, string) {
	revision := cause.RollbackRevision
	if strings.HasPrefix(r.App, "helm/") {
		if u, ok := release.HelmUndo(r.Namespace, r.App,
			cause.HelmRollbackRevision); ok {
			return u, cause.HelmRollbackRevision
		}
		_, name, _ := strings.Cut(r.App, "/")
		_, name = lastPart(name)
		return release.Undo{Text: withheldLead + "a deployed revision " +
			"of Helm release " + name + " to go back to, so it does " +
			"not suggest rolling it back"}, ""
	}
	u := release.UndoFor(r.Kind, r.Namespace, r.Name, r.App, revision)
	if managed {
		return u, ""
	}
	if !strings.Contains(u.Command, "--to-revision=") {
		return release.Undo{}, ""
	}
	return u, revision
}

// blames reports that cause blames a change of r's workload.
func blames(cause *rootcause.CauseRecord, r release.Record) bool {
	if cause == nil || cause.Change == nil {
		return false
	}
	e := cause.Change.Entity
	return strings.EqualFold(string(e.Kind), r.Kind) &&
		e.Namespace == r.Namespace && e.Name == r.Name
}

// managedBy reports a workload a GitOps controller or Helm deploys.
func managedBy(app string) bool {
	tool, _, _ := strings.Cut(app, "/")
	return tool == "argocd" || tool == "flux" || tool == "helm"
}

// lastPart splits "ns/name" into its namespace and name.
func lastPart(s string) (string, string) {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// withheldText says why no rollback is offered.
func withheldText(revision string) string {
	what := "the previous version"
	if revision != "" {
		what = "revision " + revision
	}
	return withheldLead + what + " running healthily, so it does not " +
		"suggest rolling back to it"
}
