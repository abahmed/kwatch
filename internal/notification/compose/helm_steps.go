package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// helmSteps read a failed or stuck Helm release, then offer the
// rollback to its last deployed revision, marked as a change, when the
// cause is at least likely (undoSure). The target is the newest earlier
// revision Helm recorded as deployed (now deployed or superseded), never
// simply the one before: that one may have failed too. A release a
// GitOps controller manages is fixed there: a rollback in the cluster
// is undone by its next reconcile.
func helmSteps(
	root inventory.EntityID, cause *rootcause.CauseRecord,
	members []detection.Finding,
) []notification.Step {
	name := quote(root.Name) + namespaceFlag(root.Namespace)
	// helm status prints the error Helm recorded, which is in the
	// release payload kwatch never reads, so it is named, not run.
	steps := []notification.Step{
		{Text: "See its revisions", Command: "helm history " + name},
		{Text: "See the error Helm recorded: helm status " + name},
	}
	if !undoSure(cause) {
		return steps
	}
	for _, m := range members {
		if m.Reason != reasons.HelmReleaseFailed &&
			m.Reason != reasons.HelmReleaseStuck {
			continue
		}
		good := evidence(m, detection.EvidenceHelmGoodRevision)
		if good == "" {
			break
		}
		return append(steps, notification.Step{
			Text: "Roll back to revision " + good + " (changes the " +
				"cluster); if a GitOps controller such as Flux manages " +
				"this release, change it in Git instead",
			Command: "helm rollback " + quote(root.Name) + " " +
				quote(good) + namespaceFlag(root.Namespace),
			Mutating: true,
		})
	}
	return steps
}
