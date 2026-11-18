package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/release"
)

// managedUndo is the undo step of a workload a GitOps controller or Helm
// deploys (app is "argocd/…", "flux/…" or "helm/…"): a revert in Git,
// since a rollback in the cluster would be overwritten by the next sync,
// or a Helm rollback, so the release history stays true. false for a
// workload deployed by other means, which keeps the rollout undo. A
// Helm rollback goes to helmRevision, the release's last deployed
// revision (helmUndo).
func managedUndo(
	workload inventory.EntityID, app, helmRevision string,
) (notification.Step, bool) {
	tool, _, _ := strings.Cut(app, "/")
	switch tool {
	case "argocd", "flux":
	case "helm":
		return helmUndo(workload, app, helmRevision), true
	default:
		return notification.Step{}, false
	}
	u := release.UndoFor(string(workload.Kind), workload.Namespace,
		workload.Name, app, "")
	if u.Command == "" {
		// A revert in Git: no command kwatch could offer.
		return notification.Step{Text: u.Text}, true
	}
	// A rollback always changes the cluster.
	return notification.Step{Text: u.Text, Command: u.Command,
		Mutating: true}, true
}

// managedAction is the undo a message offers for a release of a workload
// a GitOps controller or Helm deploys (managedUndo), on the same terms
// as a rollback: only when the release is at least the likely cause.
func managedAction(
	f caseFacts, steps []notification.Step,
) (string, bool) {
	if !undoSure(f.p.Cause) {
		return "", false
	}
	for _, step := range steps {
		switch {
		case step.Command == "" &&
			strings.HasPrefix(step.Text, "Revert the change in Git"):
			return step.Text + ".", true
		case step.Mutating &&
			strings.HasPrefix(step.Command, "helm rollback "):
			return "Rolling back the Helm release undoes this release " +
				"(changes the cluster): " + step.Command, true
		}
	}
	return "", false
}

// helmUndo rolls the Helm release that deploys the workload back to
// revision, the newest earlier one Helm recorded as deployed. A bare
// "helm rollback" goes to the revision just before, which may have
// failed too, so without a known deployed revision none is offered and
// the step says why.
func helmUndo(
	workload inventory.EntityID, app, revision string,
) notification.Step {
	_, name, _ := strings.Cut(app, "/")
	space := workload.Namespace
	if s, r, ok := strings.Cut(name, "/"); ok {
		space, name = s, r
	}
	if revision == "" {
		return notification.Step{Text: withheldLead + "a deployed " +
			"revision of Helm release " + name + " to go back to, so " +
			"it does not suggest rolling it back"}
	}
	return notification.Step{
		Text: "Roll back the Helm release " + name + " to revision " +
			revision + ", its last deployed one (changes the cluster)",
		Command: "helm rollback " + quote(name) + " " + quote(revision) +
			namespaceFlag(space),
		Mutating: true,
	}
}
