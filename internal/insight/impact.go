package insight

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) describeImpact(inc *model.Incident, ins *Insight) {
	var counts map[string]int
	var names map[string][]string
	if e.graph != nil {
		counts, names = e.impactReach(inc)
	}
	// Services resolved from live selectors are more reliable than graph
	// edges and are known even when the graph is empty.
	if len(inc.AffectedServices) > 0 {
		if names == nil {
			names = map[string][]string{}
		}
		if counts == nil {
			counts = map[string]int{}
		}
		for _, svc := range inc.AffectedServices {
			if !containsStr(names["service"], svc) {
				names["service"] = append(names["service"], svc)
			}
		}
		if counts["service"] < len(names["service"]) {
			counts["service"] = len(names["service"])
		}
	}
	if len(counts) == 0 {
		return
	}
	ins.Impact = formatImpactSummary(inc, counts, names)
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// impactReach is impactCounts plus the names behind the counts, so the
// summary can say which services, not just how many.
func (e *Engine) impactReach(
	inc *model.Incident,
) (map[string]int, map[string][]string) {
	keys := graphKeysForIncident(inc)
	if len(keys) == 0 {
		return nil, nil
	}
	counts := make(map[string]int)
	names := make(map[string][]string)
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		parts := strings.SplitN(k, "/", 3)
		deps := e.graph.TraverseDependents(parts[0], parts[1], parts[2])
		for _, d := range deps {
			if seen[d] {
				continue
			}
			seen[d] = true
			dp := strings.SplitN(d, "/", 3)
			kind := dp[0]
			counts[kind]++
			if len(dp) == 3 {
				names[kind] = append(names[kind], dp[2])
			}
		}
	}
	return counts, names
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
// namedKinds are listed by name in the impact summary. A reader acts on
// "services api, readify"; "2 services" only tells them to go look.
var namedKinds = []string{"service", "ingress"}

const maxNamedImpact = 4

// namedImpact renders the kinds worth naming, e.g.
// "services api, readify · ingress api".
func namedImpact(counts map[string]int, names map[string][]string) string {
	var parts []string
	for _, kind := range namedKinds {
		n := counts[kind]
		if n == 0 {
			continue
		}
		label := pluralLabel(kind)
		if n == 1 {
			label = kind
		}
		if list := format.JoinNames(names[kind], maxNamedImpact); list != "" {
			parts = append(parts, label+" "+list)
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", n, pluralLabel(kind)))
		}
	}
	return strings.Join(parts, " · ")
}

func formatImpactSummary(
	inc *model.Incident,
	counts map[string]int,
	names map[string][]string,
) string {
	named := namedImpact(counts, names)
	switch inc.Resource {
	case "node":
		pods := counts["pod"]
		rest := impactList(counts, "pod", "service", "ingress")
		if named != "" {
			if rest != "" {
				rest = named + ", " + rest
			} else {
				rest = named
			}
		}
		if pods > 0 && rest != "" {
			return fmt.Sprintf("%d pods on this node, affecting %s", pods, rest)
		}
		if pods > 0 {
			return fmt.Sprintf("%d pods on this node", pods)
		}
	case "configmap":
		if pods := counts["pod"]; pods > 0 {
			return formatDependentSummary("this configmap", pods, counts, named)
		}
	case "secret":
		if pods := counts["pod"]; pods > 0 {
			return formatDependentSummary("this secret", pods, counts, named)
		}
	case "pvc":
		if pods := counts["pod"]; pods > 0 {
			return formatDependentSummary("this pvc", pods, counts, named)
		}
	case "pod", "deployment", "statefulset", "daemonset":
		if named != "" {
			return "affects " + named
		}
	}
	if named != "" {
		return named
	}
	return impactList(counts)
}

// formatDependentSummary reports pods that reference the given resource plus
// any resources reached further downstream (services, ingresses, ...).
func formatDependentSummary(
	what string,
	pods int,
	counts map[string]int,
	named string,
) string {
	base := fmt.Sprintf("%d pod(s) reference %s", pods, what)
	rest := impactList(counts, "pod", "service", "ingress")
	if named != "" {
		if rest != "" {
			rest = named + ", " + rest
		} else {
			rest = named
		}
	}
	if rest != "" {
		return fmt.Sprintf("%s, affecting %s", base, rest)
	}
	return base
}

// impactList renders counts as "N pods, M services" skipping zero/omitted
// kinds.
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
