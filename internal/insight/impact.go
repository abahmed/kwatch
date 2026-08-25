package insight

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) describeImpact(inc *model.Incident, ins *Insight) {
	if e.graph == nil {
		return
	}
	counts := e.impactCounts(inc)
	if len(counts) == 0 {
		return
	}
	ins.Impact = formatImpactSummary(inc, counts)
}

// impactCounts walks the dependents graph (BFS) from every node the incident
// refers to and tallies the reached resources by kind in one pass.
func (e *Engine) impactCounts(inc *model.Incident) map[string]int {
	keys := graphKeysForIncident(inc)
	if len(keys) == 0 {
		return nil
	}
	counts := make(map[string]int)
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		parts := strings.SplitN(k, "/", 3)
		for _, d := range e.graph.TraverseDependents(parts[0], parts[1], parts[2]) {
			if seen[d] {
				continue
			}
			seen[d] = true
			kind := strings.SplitN(d, "/", 2)[0]
			counts[kind]++
		}
	}
	return counts
}

// impactOrder controls how impact kinds are listed; unknown kinds are appended
// at the end, so the ordering stays stable across calls.
var impactOrder = []string{
	"pod", "service", "ingress", "node", "pvc", "persistentvolume",
	"secret", "configmap", "storageclass", "serviceaccount",
	"deployment", "replicaset", "statefulset", "daemonset", "job", "cronjob",
	"horizontalpodautoscaler", "poddisruptionbudget", "endpointslice",
}

func pluralLabel(kind string) string {
	switch kind {
	case "pod":
		return "pods"
	case "service":
		return "services"
	case "ingress":
		return "ingresses"
	case "node":
		return "nodes"
	case "pvc":
		return "PVCs"
	case "persistentvolume":
		return "persistent volumes"
	case "secret":
		return "secrets"
	case "configmap":
		return "configmaps"
	case "storageclass":
		return "storage classes"
	case "serviceaccount":
		return "service accounts"
	case "deployment":
		return "deployments"
	case "replicaset":
		return "replica sets"
	case "statefulset":
		return "statefulsets"
	case "daemonset":
		return "daemonsets"
	case "job":
		return "jobs"
	case "cronjob":
		return "cronjobs"
	case "horizontalpodautoscaler":
		return "HPAs"
	case "poddisruptionbudget":
		return "PDBs"
	case "endpointslice":
		return "endpoint slices"
	}
	return kind + "s"
}

// formatImpactSummary turns the per-kind counts into a human sentence. When the
// incident refers to a "dependency producer" (node, configmap, secret, pvc) the
// sentence keeps the resource-specific phrasing and appends the downstream
// exposure so operators see the full blast radius.
func formatImpactSummary(inc *model.Incident, counts map[string]int) string {
	switch inc.Resource {
	case "node":
		pods := counts["pod"]
		rest := impactList(counts, "pod")
		if pods > 0 && rest != "" {
			return fmt.Sprintf("%d pods on this node, affecting %s", pods, rest)
		}
		if pods > 0 {
			return fmt.Sprintf("%d pods on this node", pods)
		}
	case "configmap":
		if pods := counts["pod"]; pods > 0 {
			return formatDependentSummary("this configmap", pods, counts)
		}
	case "secret":
		if pods := counts["pod"]; pods > 0 {
			return formatDependentSummary("this secret", pods, counts)
		}
	case "pvc":
		if pods := counts["pod"]; pods > 0 {
			return formatDependentSummary("this pvc", pods, counts)
		}
	case "pod", "deployment", "statefulset", "daemonset":
		if svcs := counts["service"]; svcs > 0 {
			return fmt.Sprintf("affects %d service(s)", svcs)
		}
	}
	return impactList(counts)
}

// formatDependentSummary reports pods that reference the given resource plus
// any resources reached further downstream (services, ingresses, ...).
func formatDependentSummary(what string, pods int, counts map[string]int) string {
	base := fmt.Sprintf("%d pod(s) reference %s", pods, what)
	rest := impactList(counts, "pod")
	if rest != "" {
		return fmt.Sprintf("%s, affecting %s", base, rest)
	}
	return base
}

// impactList renders counts as "N pods, M services" skipping zero/omitted kinds.
func impactList(counts map[string]int, omit ...string) string {
	skip := make(map[string]bool, len(omit))
	for _, k := range omit {
		skip[k] = true
	}
	var parts []string
	for _, kind := range impactOrder {
		if skip[kind] {
			continue
		}
		if n := counts[kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, pluralLabel(kind)))
		}
	}
	return strings.Join(parts, ", ")
}
