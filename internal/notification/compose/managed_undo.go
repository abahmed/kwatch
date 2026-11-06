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
// workload deployed by other means, which keeps the rollout undo.
func managedUndo(
	workload inventory.EntityID, app string,
) (notification.Step, bool) {
	tool, _, _ := strings.Cut(app, "/")
	switch tool {
	case "argocd", "flux", "helm":
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
	if _, sure := rollbackWords[certaintyOf(f.p.Cause)]; !sure {
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
