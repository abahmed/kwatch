package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// withheldLead starts the step and sentence that say why no rollback is
// offered for a release kwatch has no record of serving.
const withheldLead = "kwatch has no record of "

// undoSure reports a cause sure enough for kwatch to suggest undoing
// it: at least likely, as rootcause.CertaintyOf words it. Below that the
// change might be unrelated, and every undo changes the cluster, so
// only read-only checks are offered. The message's prose and its steps
// (which the dashboard stores) both ask this one predicate.
func undoSure(cause *rootcause.CauseRecord) bool {
	return certaintyOf(cause) >= certaintyLikely
}

// undoSteps are the steps about undoing the blamed release of a
// workload, after its read-only checks. The undo is offered only for a
// cause that is at least likely (undoSure) whose previous version
// kwatch saw serve (PreviousServed): to a known revision with kubectl,
// or where a GitOps controller or Helm deploys it (managedUndo).
// Below likely, the step says what would confirm the release is the
// cause; without a record of the previous version serving, it says why
// no rollback is suggested.
func undoSteps(
	workload inventory.EntityID, cause *rootcause.CauseRecord,
) []notification.Step {
	revision := cause.RollbackRevision
	if cause.Change.Entity != workload {
		// The revision is the blamed workload's; it says nothing
		// about rolling this one back.
		revision = ""
	}
	undo, managed := managedUndo(workload, cause.Change.App,
		cause.HelmRollbackRevision)
	switch {
	case !managed && revision == "":
		return nil
	case !undoSure(cause):
		return []notification.Step{confirmStep(workload, revision)}
	case !cause.PreviousServed:
		return []notification.Step{{Text: withheldText(revision)}}
	case managed:
		return []notification.Step{undo}
	}
	return []notification.Step{{
		Text: "Roll back to revision " + revision +
			" (changes the cluster)",
		Command: "kubectl rollout undo " + rolloutRef(workload) +
			namespaceFlag(workload.Namespace) +
			" --to-revision=" + quote(revision),
		Mutating: true,
	}}
}

// confirmStep is the read-only check that would confirm a release kwatch
// only suspects: what the revision before it ran, to compare with it.
func confirmStep(
	workload inventory.EntityID, revision string,
) notification.Step {
	const confirms = "; a difference that explains the failure " +
		"confirms the release is the cause"
	if revision == "" {
		return notification.Step{Text: "Compare this release with the " +
			"version before it" + confirms}
	}
	return notification.Step{
		Text: "Compare this release with revision " + revision +
			", the one before it" + confirms,
		Command: "kubectl rollout history " + rolloutRef(workload) +
			namespaceFlag(workload.Namespace) +
			" --revision=" + quote(revision),
	}
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

// withheldSentence repeats the withheld step in the prose.
func withheldSentence(steps []notification.Step) (sentence, bool) {
	for _, step := range steps {
		if step.Command == "" && strings.HasPrefix(step.Text, withheldLead) {
			return sentence{part: partAction, text: step.Text + "."},
				true
		}
	}
	return sentence{}, false
}

// rolloutRef is the workload as kubectl rollout names it, quoted.
func rolloutRef(workload inventory.EntityID) string {
	return quote(string(workload.Kind) + "/" + workload.Name)
}
