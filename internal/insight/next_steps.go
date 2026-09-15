package insight

import (
	"sort"

	"github.com/abahmed/kwatch/internal/model"
)

func nextSteps(inc *model.Incident) []string {
	if inc == nil {
		return nil
	}
	name := inc.Ref().Name
	if inc.Resource == "pod" && len(inc.Resources) > 0 {
		pods := make([]string, 0, len(inc.Resources))
		for pod := range inc.Resources {
			pods = append(pods, pod)
		}
		sort.Strings(pods)
		name = pods[0]
	}
	switch inc.Resource {
	case "pod":
		return []string{
			"kubectl describe pod " + name + namespaceArg(inc.Namespace),
			"kubectl logs " + name + namespaceArg(inc.Namespace) +
				" --all-containers",
		}
	case "node":
		return []string{
			"kubectl describe node " + name,
			"kubectl get pods -A --field-selector spec.nodeName=" + name,
		}
	case "deployment":
		return []string{
			"kubectl rollout status deployment/" + name +
				namespaceArg(inc.Namespace),
			"kubectl rollout history deployment/" + name +
				namespaceArg(inc.Namespace),
		}
	case "pvc", "persistentvolumeclaim":
		// Storage incidents use the short "pvc" resource vocabulary.
		return []string{
			"kubectl describe pvc " + name + namespaceArg(inc.Namespace),
			"kubectl get pv" + namespaceArg(inc.Namespace),
		}
	default:
		return nil
	}
}

func namespaceArg(namespace string) string {
	if namespace == "" {
		return ""
	}
	return " -n " + namespace
}
