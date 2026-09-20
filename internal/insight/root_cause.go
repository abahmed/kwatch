package insight

import (
	"fmt"
	"sort"
	"strings"

	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

// modelCauseRef is an internal graph reference annotated with traversal
// depth and evidence score. It stays private because the graph is an insight
// implementation detail, not part of the persisted diagnosis contract.
type modelCauseRef struct {
	Kind      string
	Namespace string
	Name      string
	depth     int
	score     int
}

// rootCauses walks every graph node represented by the incident and returns
// the deepest dependency resources as candidate root causes.
func (e *Engine) rootCauses(inc *model.Incident) []modelCauseRef {
	if e.graph == nil {
		return nil
	}
	var roots []modelCauseRef
	for _, key := range graphKeysForIncident(inc) {
		roots = appendRoots(roots, walkBackToRoots(e.graph, key))
	}
	if len(roots) == 0 {
		return nil
	}
	sortRoots(roots)
	return roots
}

// walkBackToRoots performs a breadth-first traversal over dependency edges.
// The deepest dead ends are the useful candidates for diagnosis.
func walkBackToRoots(
	graph *context.ResourceGraph,
	startKey string,
) []modelCauseRef {
	type graphRef struct {
		key   string
		depth int
	}
	queue := []graphRef{{key: startKey}}
	visited := map[string]bool{startKey: true}
	best := make(map[string]int)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		parts := strings.SplitN(current.key, "/", 3)
		deps := graph.DependenciesOf(parts[0], parts[1], parts[2])
		if len(deps) == 0 {
			if previous, ok := best[current.key]; !ok || current.depth > previous {
				best[current.key] = current.depth
			}
			continue
		}
		for _, dependency := range deps {
			if visited[dependency] {
				continue
			}
			visited[dependency] = true
			queue = append(queue, graphRef{
				key:   dependency,
				depth: current.depth + 1,
			})
		}
	}
	if len(best) == 0 {
		return nil
	}
	return causeRefs(best)
}

func causeRefs(best map[string]int) []modelCauseRef {
	refs := make([]modelCauseRef, 0, len(best))
	for key, depth := range best {
		parts := strings.SplitN(key, "/", 3)
		// A node lease reflects node health; it is never a root cause.
		if parts[0] == "lease" {
			continue
		}
		refs = append(refs, modelCauseRef{
			Kind: parts[0], Namespace: parts[1], Name: parts[2], depth: depth,
		})
	}
	return refs
}

func appendRoots(dst, src []modelCauseRef) []modelCauseRef {
	seen := make(map[string]bool, len(dst))
	for _, root := range dst {
		seen[rootKey(root)] = true
	}
	for _, root := range src {
		key := rootKey(root)
		if seen[key] {
			for i := range dst {
				if rootKey(dst[i]) == key && root.depth > dst[i].depth {
					dst[i].depth = root.depth
				}
			}
			continue
		}
		seen[key] = true
		dst = append(dst, root)
	}
	return dst
}

func rootKey(root modelCauseRef) string {
	return model.ObjectKey(root.Kind, root.Namespace, root.Name)
}

func sortRoots(roots []modelCauseRef) {
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].depth != roots[j].depth {
			return roots[i].depth > roots[j].depth
		}
		if roots[i].Kind != roots[j].Kind {
			return roots[i].Kind < roots[j].Kind
		}
		return roots[i].Name < roots[j].Name
	})
}

// describeRootCauses renders the root-cause sentence used by incident and
// mass-failure insight. Keep these categories stable for alert consumers.
func describeRootCauses(roots []modelCauseRef) (string, string) {
	for _, root := range roots {
		switch root.Kind {
		case "node":
			return fmt.Sprintf(
				"underlying node %s may be unhealthy", root.Name,
			), "node_failure"
		case "persistentvolume":
			return fmt.Sprintf(
				"underlying persistent volume %s may be unavailable", root.Name,
			), "storage_failure"
		case "storageclass":
			return fmt.Sprintf(
				"underlying storage class %s may be unavailable", root.Name,
			), "storage_failure"
		case "configmap":
			return fmt.Sprintf(
				"underlying configmap %s may be changed or misconfigured",
				root.Name,
			), "config_error"
		case "secret":
			return fmt.Sprintf(
				"underlying secret %s may be changed or misconfigured", root.Name,
			), "config_error"
		case "serviceaccount":
			return fmt.Sprintf(
				"underlying serviceaccount %s may be misconfigured", root.Name,
			), "config_error"
		case "endpoint":
			return fmt.Sprintf(
				"endpoint %s is not ready to receive traffic", root.Name,
			), "endpoint_failure"
		case "volumeattachment_failure":
			return fmt.Sprintf(
				"volume attachment %s reported an attach failure", root.Name,
			), "storage_attachment_failure"
		case "volumesnapshot_failure":
			return fmt.Sprintf(
				"volume snapshot %s reported an error", root.Name,
			), "storage_snapshot_failure"
		case "networktarget":
			return fmt.Sprintf(
				"network probe target %s is unreachable or unhealthy", root.Name,
			), "network_probe_failure"
		}
	}
	if len(roots) == 0 {
		return "", ""
	}
	root := roots[0]
	label := root.Name
	if root.Namespace != "" {
		label = root.Namespace + "/" + root.Name
	}
	return fmt.Sprintf(
		"underlying %s %s may be unhealthy", root.Kind, label,
	), "root_cause"
}
