package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

// dependencyChangeWindow is how far back a ConfigMap or Secret update still
// counts as a plausible cause for a workload that depends on it.
const dependencyChangeWindow = 15 * time.Minute

// reasonCauses are diagnoses that follow from the reason alone. A container
// throttled at its CPU limit is not a config error and an HPA that cannot read
// metrics is not an unhealthy Deployment; consulting the dependency graph for
// these only produced confident-sounding wrong answers.
var reasonCauses = map[string]struct{ cause, pattern string }{
	constant.ReasonContainerCPUThrottled: {
		"the container is being throttled at its CPU limit; raise the " +
			"limit or lower the request/limit gap",
		"resource_limit",
	},
	constant.ReasonContainerMemoryHigh: {
		"the container is close to its memory limit and will be OOM-killed " +
			"if usage keeps growing",
		"resource_limit",
	},
	constant.ReasonContainerCPUHigh: {
		"the container is using nearly all of its CPU limit",
		"resource_limit",
	},
	constant.ReasonNodePSIHigh: {
		"the node is stalling on CPU, memory or I/O pressure; every pod on " +
			"it is slowed",
		"node_pressure",
	},
	constant.ReasonFailedGetResourceMetric: {
		"the metrics API returned no data for the scale target; " +
			"metrics-server may be unavailable or the pods too new to report",
		"metrics_unavailable",
	},
	constant.ReasonFailedComputeMetricsReplicas: {
		"the metrics API returned no data for the scale target; " +
			"metrics-server may be unavailable or the pods too new to report",
		"metrics_unavailable",
	},
	constant.ReasonFailedGetMetrics: {
		"the metrics API returned no data for the scale target; " +
			"metrics-server may be unavailable or the pods too new to report",
		"metrics_unavailable",
	},
}

type modelCauseRef struct {
	Kind      string
	Namespace string
	Name      string
	depth     int
	score     int
}

func (e *Engine) determineCause(inc *model.Incident, ins *Insight) {
	if rc, ok := reasonCauses[inc.Reason]; ok {
		ins.Cause, ins.Pattern = rc.cause, rc.pattern
		return
	}
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

	// A referenced ConfigMap or Secret is only a suspect when it actually
	// changed recently. Every pod references some, so blaming one merely
	// for being referenced attributed nearly every incident to configuration.
	for _, d := range deps {
		switch {
		case strings.HasPrefix(d, "configmap/"):
			if !e.changedRecently(d) {
				continue
			}
			ins.Cause = "referenced ConfigMap " + shortName(d) +
				" changed shortly before this incident"
			ins.Pattern = "config_error"
			return
		case strings.HasPrefix(d, "secret/"):
			if !e.changedRecently(d) {
				continue
			}
			ins.Cause = "referenced Secret " + shortName(d) +
				" changed shortly before this incident"
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
		e.rankRootsByEvidence(roots)
		roots = e.dropUnchangedConfigRoots(roots)
		if len(roots) > 0 {
			ins.Cause, ins.Pattern = describeRootCauses(roots)
		}
	}
}

// dropUnchangedConfigRoots removes ConfigMap and Secret roots that have not
// changed recently. Being the deepest dependency is topology, not evidence:
// nearly every pod bottoms out in some ConfigMap, and blaming it for being
// there gave the same false attribution the direct check above used to.
func (e *Engine) dropUnchangedConfigRoots(
	roots []modelCauseRef,
) []modelCauseRef {
	out := make([]modelCauseRef, 0, len(roots))
	for _, r := range roots {
		if r.Kind == "configmap" || r.Kind == "secret" {
			if !e.changedRecently(
				model.ObjectKey(r.Kind, r.Namespace, r.Name),
			) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// changedRecently reports whether the tracker saw an update to the resource
// behind a "kind/namespace/name" graph key within dependencyChangeWindow.
func (e *Engine) changedRecently(depKey string) bool {
	if e.tracker == nil {
		return false
	}
	recent := e.tracker.RecentChangesBeforeAt(dependencyChangeWindow, e.now())
	for _, c := range recent {
		if c.Type != context.ChangeUpdate {
			continue
		}
		if c.Resource+"/"+c.Namespace+"/"+c.Name == depKey {
			return true
		}
	}
	return false
}

// shortName renders a "kind/namespace/name" key as "namespace/name".
func shortName(depKey string) string {
	parts := strings.SplitN(depKey, "/", 3)
	if len(parts) != 3 {
		return depKey
	}
	if parts[1] == "" {
		return parts[2]
	}
	return parts[1] + "/" + parts[2]
}

// rankRootsByEvidence makes the graph traversal time-aware. A deep dependency
// is useful, but a resource that changed immediately before the incident is a
// much stronger suspect than an unchanged leaf. This keeps topology as the
// fallback while letting recent informer changes break ties intelligently.
func (e *Engine) rankRootsByEvidence(roots []modelCauseRef) {
	for i := range roots {
		roots[i].score = roots[i].depth * 10
		if e.tracker == nil {
			continue
		}
		for _, change := range e.tracker.RecentChangesBeforeAt(10*time.Minute, e.now()) {
			if change.Resource != roots[i].Kind || change.Namespace != roots[i].Namespace ||
				change.Name != roots[i].Name {
				continue
			}
			switch change.Type {
			case context.ChangeUpdate:
				roots[i].score += recencyScore(change.Timestamp, e.now())
			case context.ChangeCreate, context.ChangeDelete:
				roots[i].score += recencyScore(change.Timestamp, e.now()) / 2
			}
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].score != roots[j].score {
			return roots[i].score > roots[j].score
		}
		return roots[i].depth > roots[j].depth
	})
}

func recencyScore(timestamp, now time.Time) int {
	age := now.Sub(timestamp)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= time.Minute:
		return 50
	case age <= 3*time.Minute:
		return 35
	case age <= 7*time.Minute:
		return 20
	default:
		return 10
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
		// A node's heartbeat lease reflects the node; it never causes
		// anything. Left in, it was the deepest "dependency" of every node
		// incident and was blamed as stale while renewing every ten seconds.
		if parts[0] == "lease" {
			continue
		}
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
		k := model.ObjectKey(r.Kind, r.Namespace, r.Name)
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
		case "endpoint":
			return fmt.Sprintf("endpoint %s is not ready to receive traffic", r.Name), "endpoint_failure"
		case "volumeattachment_failure":
			return fmt.Sprintf("volume attachment %s reported an attach failure", r.Name), "storage_attachment_failure"
		case "volumesnapshot_failure":
			return fmt.Sprintf("volume snapshot %s reported an error", r.Name), "storage_snapshot_failure"
		case "networktarget":
			return fmt.Sprintf("network probe target %s is unreachable or unhealthy", r.Name), "network_probe_failure"
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
