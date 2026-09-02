package feature

import "fmt"

// Lifecycle documents when a capability is relevant to the product. It is
// metadata for the generated catalog consumed by kwatch.sh.
type Lifecycle string

const (
	StartupOnly Lifecycle = "startup"
	Runtime     Lifecycle = "runtime"
)

// Definition is the source of truth for a capability in the generated
// catalog. Dependencies are explicit; there is no implicit inheritance.
type Definition struct {
	ID           ID
	Description  string
	Lifecycle    Lifecycle
	Dependencies []ID
}

var definitions = []Definition{
	{PodDetection, "Detect pod and container failures", Runtime, nil},
	{PodScheduling, "Detect pod scheduling failures", Runtime, []ID{PodDetection}},
	{PodPending, "Detect pods stuck Pending", Runtime, []ID{PodDetection}},
	{
		PodOOM, "Detect repeated out-of-memory failures", Runtime,
		[]ID{PodDetection},
	},
	{
		PodReadiness, "Detect sustained pod readiness failures", Runtime,
		[]ID{PodDetection},
	},
	{
		PodRestarts, "Detect excessive container restarts", Runtime,
		[]ID{PodDetection},
	},
	{
		WorkloadDetection, "Detect workload rollout and execution failures", Runtime,
		nil,
	},
	{
		DeploymentRollout, "Detect stuck Deployment rollouts", Runtime,
		[]ID{WorkloadDetection},
	},
	{
		StatefulSetRollout, "Detect stuck StatefulSet rollouts", Runtime,
		[]ID{WorkloadDetection},
	},
	{
		DaemonSetRollout, "Detect stuck DaemonSet rollouts", Runtime,
		[]ID{WorkloadDetection},
	},
	{
		JobFailures, "Detect failed and suspended Jobs", Runtime,
		[]ID{WorkloadDetection},
	},
	{
		CronJobFailures, "Detect failed and missed CronJobs", Runtime,
		[]ID{WorkloadDetection},
	},
	{
		PDBViolations, "Detect PodDisruptionBudget violations", Runtime,
		[]ID{WorkloadDetection},
	},
	{
		HPADiagnostics, "Diagnose HorizontalPodAutoscaler failures", Runtime,
		[]ID{WorkloadDetection},
	},
	{NodeDetection, "Detect node readiness and resource failures", Runtime, nil},
	{
		NodeConditions, "Detect node conditions and lifecycle failures", Runtime,
		[]ID{NodeDetection},
	},
	{NodeResources, "Detect node resource pressure", Runtime, []ID{NodeDetection}},
	{StorageDetection, "Detect persistent storage failures", Runtime, nil},
	{
		PVCUsage, "Detect PVC usage and volume failures", Runtime,
		[]ID{StorageDetection},
	},
	{NetworkDetection, "Detect service and network failures", Runtime, nil},
	{
		ServiceEndpoints, "Detect Service and EndpointSlice failures", Runtime,
		[]ID{NetworkDetection},
	},
	{
		IngressBackends, "Detect Ingress backend failures", Runtime,
		[]ID{NetworkDetection},
	},
	{
		NetworkPolicies, "Detect NetworkPolicy failures", Runtime,
		[]ID{NetworkDetection},
	},
	{SecurityDetection, "Detect security and admission failures", Runtime, nil},
	{
		AdmissionWebhooks, "Detect admission webhook failures", Runtime,
		[]ID{SecurityDetection},
	},
	{ClusterResources, "Detect cluster resource status failures", Runtime, nil},
	{
		TLSSignals, "Detect TLS certificate expiry", Runtime,
		[]ID{SecurityDetection},
	},
	{DirectDiagnosis, "Explain the most likely direct cause", Runtime, nil},
	{
		DependencyGraph, "Trace related Kubernetes dependencies", Runtime,
		[]ID{DirectDiagnosis},
	},
	{
		ImpactAnalysis, "Estimate affected resources and blast radius", Runtime,
		[]ID{DependencyGraph},
	},
	{
		ChangeDiff, "Relate incidents to recent changes", Runtime,
		[]ID{DirectDiagnosis},
	},
	{IncidentTimeline, "Keep a compact incident timeline", Runtime, nil},
	{
		RCAConfidence, "Show confidence and supporting evidence", Runtime,
		[]ID{DirectDiagnosis},
	},
	{
		RCAFeedback, "Persist operator feedback for RCA improvement", Runtime,
		[]ID{DirectDiagnosis},
	},
	{Cooldown, "Suppress repeated notifications during cooldown", Runtime, nil},
	{SmartGrouping, "Group related incidents into one narrative", Runtime, nil},
	{
		MassFailureSuppression, "Reduce noise during broad failures", Runtime,
		[]ID{SmartGrouping},
	},
	{
		CascadeSuppression, "Suppress symptoms after a root cause is known", Runtime,
		[]ID{DependencyGraph},
	},
	{
		IncidentPersistence,
		"Restore active incident lifecycle after restart", StartupOnly,
		nil,
	},
	{BaselinePersistence, "Persist startup baseline state", StartupOnly, nil},
	{ChangePersistence, "Persist recent change history", Runtime, nil},
	{KubeletTelemetry, "Read built-in kubelet summary telemetry", Runtime, nil},
	{CPUUsage, "Detect CPU usage pressure", Runtime, []ID{KubeletTelemetry}},
	{
		CPUThrottling, "Detect container CPU throttling", Runtime,
		[]ID{KubeletTelemetry},
	},
	{
		MemoryUsage, "Detect memory pressure and overuse", Runtime,
		[]ID{KubeletTelemetry},
	},
	{
		StorageUsage, "Detect ephemeral storage and inode pressure", Runtime,
		[]ID{KubeletTelemetry},
	},
	{
		PressureSignals, "Detect cgroup pressure signals", Runtime,
		[]ID{KubeletTelemetry},
	},
	{
		NetworkErrors, "Detect kubelet-observed network errors", Runtime,
		[]ID{KubeletTelemetry},
	},
	{
		RuntimeErrors, "Detect container runtime error rates", Runtime,
		[]ID{KubeletTelemetry},
	},
	{MetricsAPI, "Read the optional Kubernetes metrics API", Runtime, nil},
	{AdaptiveBaseline, "Adapt bounded thresholds to observed usage", Runtime, nil},
	{HTTPProbes, "Run configured HTTP checks", Runtime, nil},
	{TCPProbes, "Run configured TCP checks", Runtime, nil},
	{DNSProbes, "Run configured DNS checks", Runtime, nil},
	{AutomaticProbes, "Derive safe probe targets from services", Runtime, nil},
	{ProbeLatency, "Detect probe latency regressions", Runtime, nil},
	{ControlPlanePods, "Observe control-plane component pods", Runtime, nil},
	{APIServerHealth, "Check Kubernetes API health endpoints", Runtime, nil},
	{
		APIServerLatency, "Measure Kubernetes API latency", Runtime,
		[]ID{APIServerHealth},
	},
	{SchedulerHealth, "Observe scheduler health", Runtime, []ID{ControlPlanePods}},
	{
		ControllerManagerHealth, "Observe controller-manager health", Runtime,
		[]ID{ControlPlanePods},
	},
	{EtcdHealth, "Observe etcd health signals", Runtime, []ID{APIServerHealth}},
	{
		GenericStatus, "Observe status conditions on cluster resources", Runtime,
		[]ID{ClusterResources},
	},
	{
		CRDDiscovery, "Discover supported custom resources dynamically", StartupOnly,
		nil,
	},
	{RBACAudit, "Report missing permissions and RBAC drift", Runtime, nil},
	{TLSMonitoring, "Monitor configured Kubernetes TLS secrets", Runtime, nil},
	{AuditLog, "Write structured incident audit records", Runtime, nil},
	{Escalation, "Escalate incidents through alert tiers", Runtime, nil},
	{CustomTemplates, "Render operator-selected alert templates", Runtime, nil},
	{Runbooks, "Attach reason-aware runbook links", Runtime, nil},
}

// Catalog returns a copy so callers cannot mutate the product registry.
func Catalog() []Definition {
	result := make([]Definition, len(definitions))
	copy(result, definitions)
	for i := range result {
		result[i].Dependencies = append([]ID(nil), result[i].Dependencies...)
	}
	return result
}

// Lookup returns a definition and whether the ID is known.
func Lookup(id ID) (Definition, bool) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

// ValidateCatalog rejects duplicate IDs, missing dependencies, and dependency
// cycles.
func ValidateCatalog() error {
	known := make(map[ID]Definition, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" {
			return fmt.Errorf("feature catalog contains an empty id")
		}
		if _, exists := known[definition.ID]; exists {
			return fmt.Errorf("feature catalog contains duplicate id %q", definition.ID)
		}
		known[definition.ID] = definition
	}
	state := make(map[ID]uint8, len(known))
	var visit func(ID) error
	visit = func(id ID) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("feature catalog dependency cycle at %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range known[id].Dependencies {
			if _, exists := known[dependency]; !exists {
				return fmt.Errorf("feature %q depends on unknown feature %q", id, dependency)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range known {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
