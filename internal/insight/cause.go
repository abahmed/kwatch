package insight

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

type modelCauseRef struct {
	Kind      string
	Namespace string
	Name      string
	depth     int
}

func (e *Engine) determineCause(inc *model.Incident, ins *Insight) {
	if e.graph == nil {
		return
	}
	ns := inc.Namespace

	deps := dependenciesFor(e.graph, inc)
	if len(deps) == 0 {
		return
	}

	nodeKey := "node//" + inc.NodeName
	if inc.NodeName != "" {
		for _, d := range deps {
			if d == nodeKey {
				ins.Cause = fmt.Sprintf("node %s may be unhealthy", inc.NodeName)
				ins.Pattern = "node_failure"
				return
			}
		}
	}

	if inc.Resource == "pod" && inc.OwnerKind != "" {
		ownerPrefix := strings.ToLower(inc.OwnerKind) + "/" + ns + "/"
		for _, d := range deps {
			if strings.HasPrefix(d, ownerPrefix) {
				ownerName := d[len(ownerPrefix):]
				ins.Cause = fmt.Sprintf("owning %s %s is unhealthy", inc.OwnerKind, ownerName)
				ins.Pattern = "rollout_failure"
				return
			}
		}
	}

	for _, d := range deps {
		switch {
		case strings.HasPrefix(d, "configmap/"):
			ins.Cause = "referenced ConfigMap may have changed or is misconfigured"
			ins.Pattern = "config_error"
			return
		case strings.HasPrefix(d, "secret/"):
			ins.Cause = "referenced Secret may have changed or is misconfigured"
			ins.Pattern = "config_error"
			return
		case strings.HasPrefix(d, "pvc/"):
			ins.Cause = "referenced PVC may be unavailable"
			ins.Pattern = "config_error"
			return
		}
	}

	// None of the direct dependencies match a known root cause category, so
	// walk the full transitive chain backward and blame its deepest resource.
	if roots := e.rootCauses(inc); len(roots) > 0 {
		ins.Cause, ins.Pattern = describeRootCauses(roots)
	}
}

// rootCauses walks the dependency edges backward (BFS) from every node the
// incident refers to and returns the nodes that are reached last — the deepest
// dependencies — each annotated with the BFS depth at which it was found. The
// result is ordered by depth (deepest first), so the primary suspect comes
// first. Producer resources that commonly sit at the bottom of the chain
// (node, persistentvolume, storageclass, configmap, secret, serviceaccount,
// service) are the ones that can still surface a root cause sentence.
func (e *Engine) rootCauses(inc *model.Incident) []modelCauseRef {
	if e.graph == nil {
		return nil
	}
	var roots []modelCauseRef
	for _, k := range graphKeysForIncident(inc) {
		roots = appendRoots(roots, walkBackToRoots(e.graph, k))
	}
	if len(roots) == 0 {
		return nil
	}
	sortRoots(roots)
	return roots
}

// walkBackToRoots performs a BFS over the dependency edges starting at the
// given node key and returns the dead-end nodes reached at the deepest depth,
// plus their BFS depth (used for ordering the primary suspect first).
func walkBackToRoots(g *context.ResourceGraph, startKey string) []modelCauseRef {
	type ref struct {
		key   string
		depth int
	}
	queue := []ref{{key: startKey, depth: 0}}
	visited := map[string]bool{startKey: true}
	best := make(map[string]int)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		childParts := strings.SplitN(cur.key, "/", 3)
		deps := g.DependenciesOf(childParts[0], childParts[1], childParts[2])
		if len(deps) == 0 {
			// dead end — record as a potential root
			if prev, ok := best[cur.key]; !ok || cur.depth > prev {
				best[cur.key] = cur.depth
			}
			continue
		}
		for _, d := range deps {
			if visited[d] {
				continue
			}
			visited[d] = true
			queue = append(queue, ref{key: d, depth: cur.depth + 1})
		}
	}
	if len(best) == 0 {
		return nil
	}
	out := make([]modelCauseRef, 0, len(best))
	for k, depth := range best {
		parts := strings.SplitN(k, "/", 3)
		out = append(out, modelCauseRef{Kind: parts[0], Namespace: parts[1], Name: parts[2], depth: depth})
	}
	return out
}

func appendRoots(dst, src []modelCauseRef) []modelCauseRef {
	seen := make(map[string]bool, len(dst))
	for _, r := range dst {
		seen[r.Kind+"/"+r.Namespace+"/"+r.Name] = true
	}
	for _, r := range src {
		k := r.Kind + "/" + r.Namespace + "/" + r.Name
		if seen[k] {
			// keep the deepest depth if it was already recorded shallower
			for i := range dst {
				if dst[i].Kind == r.Kind && dst[i].Namespace == r.Namespace && dst[i].Name == r.Name && r.depth > dst[i].depth {
					dst[i].depth = r.depth
				}
			}
			continue
		}
		seen[k] = true
		dst = append(dst, r)
	}
	return dst
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

// describeRootCauses renders a root-cause sentence, preferring the most common
// failure categories.
func describeRootCauses(roots []modelCauseRef) (string, string) {
	for _, r := range roots {
		switch r.Kind {
		case "node":
			return fmt.Sprintf("underlying node %s may be unhealthy", r.Name), "node_failure"
		case "persistentvolume":
			return fmt.Sprintf("underlying persistent volume %s may be unavailable", r.Name), "storage_failure"
		case "storageclass":
			return fmt.Sprintf("underlying storage class %s may be unavailable", r.Name), "storage_failure"
		case "configmap":
			return fmt.Sprintf("underlying configmap %s may be changed or misconfigured", r.Name), "config_error"
		case "secret":
			return fmt.Sprintf("underlying secret %s may be changed or misconfigured", r.Name), "config_error"
		case "serviceaccount":
			return fmt.Sprintf("underlying serviceaccount %s may be misconfigured", r.Name), "config_error"
		}
	}
	// fallback: name the deepest overall resource
	r := roots[0]
	label := r.Name
	if r.Namespace != "" {
		label = r.Namespace + "/" + r.Name
	}
	return fmt.Sprintf("underlying %s %s may be unhealthy", r.Kind, label), "root_cause"
}
