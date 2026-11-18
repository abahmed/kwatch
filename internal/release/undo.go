package release

import (
	"strconv"
	"strings"
)

// Undo is how to go back on a release: a text and, when a command does
// it, the command. Mutating is set when the command changes the cluster.
type Undo struct {
	Text     string
	Command  string
	Mutating bool
}

// UndoFor says how to undo a release of the workload kind/namespace/name
// that app deploys ("argocd/shop", "flux/apps", "helm/api", "helm/ops/api"
// for a Helm release in namespace ops, or ""), back to revision when it
// is known. A workload a GitOps controller manages is reverted in Git: a
// rollback in the cluster would be overwritten by the next sync. A Helm
// release is rolled back with Helm in its own namespace, so its history
// stays true. Anything else, a chart rendered without a release
// ("helm-chart/…") included, is rolled back with kubectl.
func UndoFor(kind, namespace, name, app, revision string) Undo {
	tool, appName, _ := strings.Cut(app, "/")
	switch tool {
	case "argocd", "flux":
		return Undo{Text: "Revert the change in Git (" + app + "): " +
			"a rollback in the cluster would be overwritten by the next sync"}
	case "helm":
		space := namespace
		if s, rel, ok := strings.Cut(appName, "/"); ok {
			space, appName = s, rel
		}
		if appName != "" {
			return Undo{Text: "Roll back the Helm release " + appName +
				" (changes the cluster)",
				Command:  "helm rollback " + appName + nsFlag(space),
				Mutating: true}
		}
	}
	command := "kubectl rollout undo " + strings.ToLower(kind) + "/" +
		name + nsFlag(namespace)
	text := "Roll back (changes the cluster)"
	if revision != "" {
		if _, err := strconv.Atoi(revision); err == nil {
			command += " --to-revision=" + revision
			text = "Roll back to revision " + revision +
				" (changes the cluster)"
		}
	}
	return Undo{Text: text, Command: command, Mutating: true}
}

func nsFlag(namespace string) string {
	if namespace == "" {
		return ""
	}
	return " -n " + namespace
}

// HelmUndo rolls the Helm release that app names ("helm/api" or
// "helm/ops/api"; namespace is the workload's when app names none) back
// to revision, the one Helm last recorded as deployed. A bare helm
// rollback goes to the revision just before, which may have failed too,
// so without a revision it offers nothing: ok is false, and also for an
// app that is not a Helm release.
func HelmUndo(namespace, app, revision string) (Undo, bool) {
	tool, name, _ := strings.Cut(app, "/")
	if s, rel, ok := strings.Cut(name, "/"); ok {
		namespace, name = s, rel
	}
	if tool != "helm" || name == "" || revision == "" {
		return Undo{}, false
	}
	return Undo{Text: "Roll back the Helm release " + name +
		" to revision " + revision + ", its last deployed one " +
		"(changes the cluster)",
		Command: "helm rollback " + name + " " + revision +
			nsFlag(namespace),
		Mutating: true}, true
}
