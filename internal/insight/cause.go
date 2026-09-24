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
		"the HPA could not obtain the resource metrics required to calculate " +
			"replicas",
		"metrics_unavailable",
	},
	constant.ReasonFailedComputeMetricsReplicas: {
		"the HPA could not obtain the metrics required to calculate replicas",
		"metrics_unavailable",
	},
	constant.ReasonFailedGetMetrics: {
		"the HPA could not obtain the metrics required to calculate replicas",
		"metrics_unavailable",
	},
}

func (e *Engine) determineCause(inc *model.Incident, ins *Insight) {
	if determineStructuredEventCause(inc, ins) {
		return
	}
	if determineMetricsCause(inc, ins) {
		return
	}
	if rc, ok := reasonCauses[inc.Reason]; ok {
		ins.Cause, ins.Pattern = rc.cause, rc.pattern
		return
	}
	e.determineGraphCause(inc, ins)
}

func (e *Engine) determineGraphCause(inc *model.Incident, ins *Insight) {
	if e.graph == nil {
		return
	}
	deps := dependenciesFor(e.graph, inc)
	if len(deps) == 0 {
		return
	}
	if determineNamedDependencyCause(inc, ins, deps) {
		return
	}
	if e.determineDirectDependencyCause(ins, deps) {
		return
	}
	roots := e.rootCauses(inc)
	if len(roots) == 0 {
		return
	}
	e.rankRootsByEvidence(roots)
	roots = e.dropUnchangedConfigRoots(roots)
	if len(roots) > 0 {
		ins.Cause, ins.Pattern = describeRootCauses(roots)
	}
}

func determineNamedDependencyCause(
	inc *model.Incident,
	ins *Insight,
	deps []string,
) bool {
	if inc.NodeName != "" {
		nodeKey := "node//" + inc.NodeName
		for _, dependency := range deps {
			if dependency == nodeKey {
				ins.Cause = fmt.Sprintf(
					"node %s may be unhealthy", inc.NodeName,
				)
				ins.Pattern = "node_failure"
				return true
			}
		}
	}
	if inc.Resource != "pod" || inc.OwnerKind == "" {
		return false
	}
	ownerPrefix := strings.ToLower(inc.OwnerKind) + "/" + inc.Namespace + "/"
	for _, dependency := range deps {
		if strings.HasPrefix(dependency, ownerPrefix) {
			ownerName := dependency[len(ownerPrefix):]
			ins.Cause = fmt.Sprintf(
				"owning %s %s is unhealthy", inc.OwnerKind, ownerName,
			)
			ins.Pattern = "rollout_failure"
			return true
		}
	}
	return false
}

func (e *Engine) determineDirectDependencyCause(
	ins *Insight,
	deps []string,
) bool {
	for _, dependency := range deps {
		switch {
		case strings.HasPrefix(dependency, "configmap/"):
			if e.changedRecently(dependency) {
				ins.Cause = "referenced ConfigMap " + shortName(dependency) +
					" changed shortly before this incident"
				ins.Pattern = "config_error"
				return true
			}
		case strings.HasPrefix(dependency, "secret/"):
			if e.changedRecently(dependency) {
				ins.Cause = "referenced Secret " + shortName(dependency) +
					" changed shortly before this incident"
				ins.Pattern = "config_error"
				return true
			}
		case strings.HasPrefix(dependency, "pvc/"):
			ins.Cause = "referenced PVC may be unavailable"
			ins.Pattern = "config_error"
			return true
		}
	}
	return false
}

func determineStructuredEventCause(
	inc *model.Incident,
	ins *Insight,
) bool {
	if inc == nil || inc.Facts.FailureDomain == "" {
		return false
	}
	dependency := inc.Facts.Dependency.Describe()
	if dependency == "" {
		dependency = "dependency"
	}
	cause, ok := structuredFailureCauses[inc.Facts.FailureCode]
	if !ok {
		return false
	}
	ins.Cause = cause
	if inc.Facts.FailureCode == "dependency_missing" ||
		inc.Facts.FailureCode == "access_denied" {
		ins.Cause = fmt.Sprintf(cause, dependency)
	}
	ins.Pattern = inc.Facts.FailureDomain + "_failure"
	return true
}

var structuredFailureCauses = map[string]string{
	"multi_attach":        "the volume is already attached to another node",
	"dependency_missing":  "a required %s does not exist",
	"access_denied":       "Kubernetes was denied access to %s",
	"insufficient_cpu":    "no eligible node has enough unallocated CPU",
	"insufficient_memory": "no eligible node has enough unallocated memory",
	"untolerated_taint": "the pod does not tolerate the taints on " +
		"eligible nodes",
	"placement_constraints": "node selection or affinity rules exclude " +
		"available nodes",
	"volume_unbound": "the pod is waiting for a persistent volume claim " +
		"to bind",
	"webhook_no_endpoints": "the admission webhook service has 0 healthy " +
		"endpoints",
	"webhook_tls": "the admission webhook TLS connection failed",
	"webhook_timeout": "the admission webhook did not respond before " +
		"its deadline",
	"network_unavailable": "the Kubernetes network is not ready",
	"discovery_failed":    "Kubernetes API discovery failed",
	"quota_exceeded": "namespace quota prevented Kubernetes from creating " +
		"the resource",
	"resource_create_failed": "Kubernetes could not create a required " +
		"workload resource",
	"scale_operation_failed": "Kubernetes could not apply the requested " +
		"replica count",
	"validation_failed": "the resource configuration failed Kubernetes validation",
	"node_not_ready":    "the node or its kubelet is not ready",
	"readiness_probe_failed": "the readiness probe is failing, so the pod " +
		"cannot receive traffic",
	"liveness_probe_failed": "the liveness probe is failing and Kubernetes " +
		"may restart the container",
	"startup_probe_failed": "the startup probe is failing before the " +
		"application becomes ready",
	"health_check_failed": "a Kubernetes health check is failing",
	"timeout":             "the Kubernetes operation exceeded its deadline",
	"storage_operation_failed": "Kubernetes could not complete the required " +
		"storage operation",
	"no_eligible_node": "no node currently satisfies the pod's scheduling " +
		"requirements",
	"webhook_call_failed": "the admission webhook rejected or failed the request",
	"network_update_failed": "Kubernetes could not update the workload's " +
		"network endpoints",
	"workload_deadline_exceeded": "the workload exhausted its retry or " +
		"execution deadline",
	"node_group_at_max": "the cluster autoscaler cannot add nodes because " +
		"the node group is already at its maximum size",
	"no_scale_up_option": "the cluster autoscaler found no node group that " +
		"can run the pending workload",
	"scale_up_backoff": "the cluster autoscaler is backing off after a " +
		"failed scale-up attempt",
	"scale_up_failed": "the cluster autoscaler could not add capacity",
}

func determineMetricsCause(inc *model.Incident, ins *Insight) bool {
	if inc == nil {
		return false
	}
	switch inc.Reason {
	case constant.ReasonFailedGetResourceMetric,
		constant.ReasonFailedComputeMetricsReplicas,
		constant.ReasonFailedGetMetrics:
	default:
		return false
	}
	metric := inc.Facts.MetricName
	if metric == "" {
		metric = "required"
	}
	switch inc.Facts.MetricFailure {
	case "api_unavailable":
		ins.Cause = "the Kubernetes Metrics API is unavailable"
		ins.Pattern = "metrics_api_failure"
	case "missing_request":
		ins.Cause = fmt.Sprintf(
			"the HPA cannot calculate %s utilization because container %s "+
				"in pod %s has no matching resource request",
			metric, inc.Facts.MetricContainer, inc.Facts.MetricPod,
		)
		ins.Pattern = "hpa_missing_request"
	case "missing_pod_metrics":
		ins.Cause = "the HPA did not receive usable metrics for its target pods"
		ins.Pattern = "hpa_missing_pod_metrics"
	case "invalid_metric":
		ins.Cause = "the HPA metric configuration is invalid"
		ins.Pattern = "hpa_invalid_metric"
	default:
		ins.Cause = "the HPA could not obtain the metrics required to " +
			"calculate replicas"
		ins.Pattern = "metrics_unavailable"
	}
	return true
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
