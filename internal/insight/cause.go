package insight

import (
	"fmt"
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
