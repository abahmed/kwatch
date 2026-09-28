package story

import (
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

// nextSteps suggests read-only commands for the real objects first, then
// clearly labelled changes.
func nextSteps(p problem.Problem, members []signal.Signal) []Step {
	root := p.Root
	switch {
	case p.Cause != nil && p.Cause.Change != nil &&
		isWorkload(root.Kind):
		return rolloutSteps(root)
	case root.Kind == kube.KindNode:
		return nodeSteps(root)
	case root.Kind == kube.KindSecret || root.Kind == kube.KindConfigMap:
		return []Step{{
			Text: "Check the object the pods depend on",
			Command: "kubectl get " + string(root.Kind) + " " + root.Name +
				" -n " + root.Namespace + " -o yaml",
		}}
	case root.Kind == kube.KindPVC:
		return []Step{{
			Text: "See why the volume claim is not usable",
			Command: "kubectl describe pvc " + root.Name + " -n " +
				root.Namespace,
		}}
	}
	return symptomSteps(members)
}

func rolloutSteps(workload knowledge.EntityID) []Step {
	ref := string(workload.Kind) + "/" + workload.Name
	ns := " -n " + workload.Namespace
	return []Step{
		{Text: "See the rollout state",
			Command: "kubectl rollout status " + ref + ns},
		{Text: "Roll back to the previous revision (changes the cluster)",
			Command: "kubectl rollout undo " + ref + ns, Mutating: true},
	}
}

func nodeSteps(node knowledge.EntityID) []Step {
	return []Step{
		{Text: "Check the node's conditions and recent events",
			Command: "kubectl describe node " + node.Name},
		{Text: "List the pods running on it",
			Command: "kubectl get pods -A -o wide --field-selector " +
				"spec.nodeName=" + node.Name},
	}
}

func symptomSteps(members []signal.Signal) []Step {
	if len(members) == 0 {
		return nil
	}
	s := members[0]
	id := s.Entity
	if id.Kind == kube.KindContainer {
		pod, container := splitContainer(id.Name)
		steps := []Step{{
			Text: "Read the output of the last crash",
			Command: "kubectl logs " + pod + " -c " + container + " -n " +
				id.Namespace + " --previous",
		}}
		if s.Reason == constant.ReasonOOMKilled {
			steps = append(steps, Step{
				Text: "Compare memory use with the limit",
				Command: "kubectl top pod " + pod + " -n " + id.Namespace +
					" --containers",
			})
		}
		return steps
	}
	return []Step{{
		Text: "See the object's state and events",
		Command: "kubectl describe " + string(id.Kind) + " " + id.Name +
			namespaceFlag(id.Namespace),
	}}
}

func isWorkload(kind knowledge.Kind) bool {
	switch kind {
	case kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet:
		return true
	}
	return false
}

func splitContainer(name string) (string, string) {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[:i], name[i+1:]
		}
	}
	return name, ""
}

func namespaceFlag(namespace string) string {
	if namespace == "" {
		return ""
	}
	return " -n " + namespace
}
