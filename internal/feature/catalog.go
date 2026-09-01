package feature

import "fmt"

// Tier describes the minimum product tier that may grant a capability.
type Tier uint8

const (
	Community Tier = iota
	Pro
)

// Lifecycle documents when a capability is evaluated. It is metadata for
// status/reporting and for future hot-reload work; current kwatch applies a
// plan at startup and never changes it behind a running monitor.
type Lifecycle string

const (
	StartupOnly Lifecycle = "startup"
	Runtime     Lifecycle = "runtime"
)

// Definition is the source of truth for a capability. Dependencies are
// explicit; there is no implicit parent-module inheritance.
type Definition struct {
	ID           ID
	Description  string
	Tier         Tier
	Lifecycle    Lifecycle
	Dependencies []ID
}

var definitions = []Definition{
	{PodDetection, "Detect pod and container failures", Community, Runtime, nil},
	{PodScheduling, "Detect pod scheduling failures", Community, Runtime, []ID{PodDetection}},
	{PodPending, "Detect pods stuck Pending", Community, Runtime, []ID{PodDetection}},
	{PodOOM, "Detect repeated out-of-memory failures", Community, Runtime, []ID{PodDetection}},
	{PodReadiness, "Detect sustained pod readiness failures", Community, Runtime, []ID{PodDetection}},
	{PodRestarts, "Detect excessive container restarts", Community, Runtime, []ID{PodDetection}},
	{PodLifecycle, "Detect pod lifecycle and termination failures", Community, Runtime, []ID{PodDetection}},
	{WorkloadDetection, "Detect workload rollout and execution failures", Community, Runtime, nil},
	{DeploymentRollout, "Detect stuck Deployment rollouts", Community, Runtime, []ID{WorkloadDetection}},
	{StatefulSetRollout, "Detect stuck StatefulSet rollouts", Community, Runtime, []ID{WorkloadDetection}},
	{DaemonSetRollout, "Detect stuck DaemonSet rollouts", Community, Runtime, []ID{WorkloadDetection}},
	{JobFailures, "Detect failed and suspended Jobs", Community, Runtime, []ID{WorkloadDetection}},
	{CronJobFailures, "Detect failed and missed CronJobs", Community, Runtime, []ID{WorkloadDetection}},
	{PDBViolations, "Detect PodDisruptionBudget violations", Community, Runtime, []ID{WorkloadDetection}},
	{HPADiagnostics, "Diagnose HorizontalPodAutoscaler failures", Community, Runtime, []ID{WorkloadDetection}},
	{NodeDetection, "Detect node readiness and resource failures", Community, Runtime, nil},
	{NodeConditions, "Detect node conditions and lifecycle failures", Community, Runtime, []ID{NodeDetection}},
	{NodeResources, "Detect node resource pressure", Community, Runtime, []ID{NodeDetection}},
	{StorageDetection, "Detect persistent storage failures", Community, Runtime, nil},
	{PVCUsage, "Detect PVC usage and volume failures", Community, Runtime, []ID{StorageDetection}},
	{NetworkDetection, "Detect service and network failures", Community, Runtime, nil},
	{ServiceEndpoints, "Detect Service and EndpointSlice failures", Community, Runtime, []ID{NetworkDetection}},
	{IngressBackends, "Detect Ingress backend failures", Community, Runtime, []ID{NetworkDetection}},
	{NetworkPolicies, "Detect NetworkPolicy failures", Community, Runtime, []ID{NetworkDetection}},
	{SecurityDetection, "Detect security and admission failures", Community, Runtime, nil},
	{AdmissionWebhooks, "Detect admission webhook failures", Community, Runtime, []ID{SecurityDetection}},
	{ClusterResources, "Detect cluster resource status failures", Community, Runtime, nil},
	{TLSSignals, "Detect TLS certificate expiry", Community, Runtime, []ID{SecurityDetection}},
	{DirectDiagnosis, "Explain the most likely direct cause", Community, Runtime, nil},
	{DependencyGraph, "Trace related Kubernetes dependencies", Community, Runtime, []ID{DirectDiagnosis}},
	{ImpactAnalysis, "Estimate affected resources and blast radius", Community, Runtime, []ID{DependencyGraph}},
	{ChangeDiff, "Relate incidents to recent changes", Community, Runtime, []ID{DirectDiagnosis}},
	{IncidentTimeline, "Keep a compact incident timeline", Community, Runtime, nil},
	{RCAConfidence, "Show confidence and supporting evidence", Community, Runtime, []ID{DirectDiagnosis}},
	{RCAFeedback, "Persist operator feedback for RCA improvement", Community, Runtime, []ID{DirectDiagnosis}},
	{Fingerprinting, "Keep incident identity stable across replacements", Community, Runtime, nil},
	{ConfirmationWindow, "Confirm transient signals before alerting", Community, Runtime, nil},
	{Cooldown, "Suppress repeated notifications during cooldown", Community, Runtime, nil},
	{SmartGrouping, "Group related incidents into one narrative", Community, Runtime, nil},
	{MassFailureSuppression, "Reduce noise during broad failures", Community, Runtime, []ID{SmartGrouping}},
	{CascadeSuppression, "Suppress symptoms after a root cause is known", Community, Runtime, []ID{DependencyGraph}},
	{SeverityByBlastRadius, "Set severity from estimated impact", Community, Runtime, []ID{ImpactAnalysis}},
	{IncidentPersistence, "Restore active incident lifecycle after restart", Community, StartupOnly, nil},
	{BaselinePersistence, "Persist startup baseline state", Community, StartupOnly, nil},
	{ChangePersistence, "Persist recent change history", Community, Runtime, nil},
	{KubeletTelemetry, "Read built-in kubelet summary telemetry", Community, Runtime, nil},
	{CPUUsage, "Detect CPU usage pressure", Community, Runtime, []ID{KubeletTelemetry}},
	{CPUThrottling, "Detect container CPU throttling", Community, Runtime, []ID{KubeletTelemetry}},
	{MemoryUsage, "Detect memory pressure and overuse", Community, Runtime, []ID{KubeletTelemetry}},
	{StorageUsage, "Detect ephemeral storage and inode pressure", Community, Runtime, []ID{KubeletTelemetry}},
	{PressureSignals, "Detect cgroup pressure signals", Community, Runtime, []ID{KubeletTelemetry}},
	{NetworkErrors, "Detect kubelet-observed network errors", Community, Runtime, []ID{KubeletTelemetry}},
	{RuntimeErrors, "Detect container runtime error rates", Community, Runtime, []ID{KubeletTelemetry}},
	{MetricsAPI, "Read the optional Kubernetes metrics API", Community, Runtime, nil},
	{AdaptiveBaseline, "Adapt bounded thresholds to observed usage", Community, Runtime, nil},
	{HTTPProbes, "Run configured HTTP checks", Community, Runtime, nil},
	{TCPProbes, "Run configured TCP checks", Community, Runtime, nil},
	{DNSProbes, "Run configured DNS checks", Community, Runtime, nil},
	{AutomaticProbes, "Derive safe probe targets from services", Community, Runtime, nil},
	{ProbeLatency, "Detect probe latency regressions", Community, Runtime, nil},
	{ProbeHeaders, "Send custom headers with HTTP probes", Community, Runtime, []ID{HTTPProbes}},
	{ControlPlanePods, "Observe control-plane component pods", Community, Runtime, nil},
	{APIServerHealth, "Check Kubernetes API health endpoints", Community, Runtime, nil},
	{APIServerLatency, "Measure Kubernetes API latency", Community, Runtime, []ID{APIServerHealth}},
	{SchedulerHealth, "Observe scheduler health", Community, Runtime, []ID{ControlPlanePods}},
	{ControllerManagerHealth, "Observe controller-manager health", Community, Runtime, []ID{ControlPlanePods}},
	{EtcdHealth, "Observe etcd health signals", Community, Runtime, []ID{APIServerHealth}},
	{InformerHealth, "Report informer sync and watch health", Community, Runtime, nil},
	{GenericStatus, "Observe status conditions on cluster resources", Community, Runtime, []ID{ClusterResources}},
	{CRDDiscovery, "Discover supported custom resources dynamically", Community, StartupOnly, nil},
	{EventFailureSecurity, "Detect security-relevant Kubernetes events", Community, Runtime, nil},
	{RBACAudit, "Report missing permissions and RBAC drift", Community, Runtime, nil},
	{AdmissionFailures, "Detect admission webhook and policy failures", Community, Runtime, nil},
	{TLSMonitoring, "Monitor configured Kubernetes TLS secrets", Community, Runtime, nil},
	{AuditLog, "Write structured incident audit records", Community, Runtime, nil},
	{SingleDestination, "Deliver alerts to one configured destination", Community, Runtime, nil},
	{MultiDestination, "Deliver alerts to multiple destinations", Community, Runtime, nil},
	{AdvancedRouting, "Route alerts with advanced policies", Pro, Runtime, []ID{MultiDestination}},
	{Escalation, "Escalate incidents through alert tiers", Community, Runtime, nil},
	{CustomTemplates, "Render operator-selected alert templates", Community, Runtime, nil},
	{Runbooks, "Attach reason-aware runbook links", Community, Runtime, nil},
	{PremiumActiveProbes, "Premium active probing package", Pro, Runtime, []ID{HTTPProbes, TCPProbes, DNSProbes}},
	{PremiumRCAFeedback, "Premium RCA feedback workflow", Pro, Runtime, []ID{RCAFeedback}},
	{PremiumExports, "Export incident reports", Pro, Runtime, nil},
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

// ValidateCatalog protects the policy boundary from accidental duplicate IDs,
// missing dependencies, or dependency cycles during development.
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
