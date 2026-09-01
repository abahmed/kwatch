// Package feature contains the product capability catalog and entitlement
// planner. It deliberately has no dependency on configuration or Kubernetes;
// the composition root translates runtime configuration into a plan.
package feature

// ID is a stable capability identifier. IDs are API, persistence, and support
// vocabulary: once shipped, an ID must not be silently renamed.
type ID string

const (
	// Core detection.
	PodDetection       ID = "core.detection.pods"
	PodScheduling      ID = "core.pods.scheduling"
	PodPending         ID = "core.pods.pending"
	PodOOM             ID = "core.pods.oom"
	PodReadiness       ID = "core.pods.readiness"
	PodRestarts        ID = "core.pods.restarts"
	PodLifecycle       ID = "core.pods.lifecycle"
	WorkloadDetection  ID = "core.detection.workloads"
	DeploymentRollout  ID = "core.workloads.deployment-rollout"
	StatefulSetRollout ID = "core.workloads.statefulset-rollout"
	DaemonSetRollout   ID = "core.workloads.daemonset-rollout"
	JobFailures        ID = "core.workloads.job-failures"
	CronJobFailures    ID = "core.workloads.cronjob-failures"
	PDBViolations      ID = "core.workloads.pdb-violations"
	HPADiagnostics     ID = "core.workloads.hpa-diagnostics"
	NodeDetection      ID = "core.detection.nodes"
	NodeConditions     ID = "core.nodes.conditions"
	NodeResources      ID = "core.nodes.resources"
	StorageDetection   ID = "core.detection.storage"
	PVCUsage           ID = "core.storage.pvc-usage"
	NetworkDetection   ID = "core.detection.network"
	ServiceEndpoints   ID = "core.network.service-endpoints"
	IngressBackends    ID = "core.network.ingress-backends"
	NetworkPolicies    ID = "core.network.policies"
	SecurityDetection  ID = "core.detection.security"
	AdmissionWebhooks  ID = "core.security.admission-webhooks"
	ClusterResources   ID = "core.cluster-resources.status"
	TLSSignals         ID = "core.security.tls"

	// Intelligence and incident lifecycle.
	DirectDiagnosis  ID = "intelligence.diagnosis.direct"
	DependencyGraph  ID = "intelligence.diagnosis.dependency-graph"
	ImpactAnalysis   ID = "intelligence.diagnosis.impact"
	ChangeDiff       ID = "intelligence.diagnosis.change-diff"
	IncidentTimeline ID = "intelligence.diagnosis.timeline"
	RCAConfidence    ID = "intelligence.diagnosis.confidence"
	RCAFeedback      ID = "intelligence.diagnosis.feedback"

	Fingerprinting         ID = "incidents.identity.fingerprint"
	ConfirmationWindow     ID = "incidents.lifecycle.confirmation"
	Cooldown               ID = "incidents.lifecycle.cooldown"
	SmartGrouping          ID = "incidents.lifecycle.grouping"
	MassFailureSuppression ID = "incidents.lifecycle.mass-failure"
	CascadeSuppression     ID = "incidents.lifecycle.cascade-suppression"
	SeverityByBlastRadius  ID = "incidents.lifecycle.blast-radius-severity"
	IncidentPersistence    ID = "incidents.persistence.active"
	BaselinePersistence    ID = "incidents.persistence.baseline"
	ChangePersistence      ID = "incidents.persistence.change-history"

	// Built-in Kubernetes telemetry and metrics API integration.
	KubeletTelemetry ID = "telemetry.kubelet.summary"
	CPUUsage         ID = "telemetry.cpu.usage"
	CPUThrottling    ID = "telemetry.cpu.throttling"
	MemoryUsage      ID = "telemetry.memory.usage"
	StorageUsage     ID = "telemetry.storage.usage"
	PressureSignals  ID = "telemetry.pressure"
	NetworkErrors    ID = "telemetry.network.errors"
	RuntimeErrors    ID = "telemetry.runtime.errors"
	MetricsAPI       ID = "telemetry.metrics-api"
	AdaptiveBaseline ID = "telemetry.adaptive-baseline"

	// Optional active checks. These are intentionally separate from passive
	// Kubernetes observation because they create user-selected traffic.
	HTTPProbes      ID = "probes.http"
	TCPProbes       ID = "probes.tcp"
	DNSProbes       ID = "probes.dns"
	AutomaticProbes ID = "probes.services.automatic"
	ProbeLatency    ID = "probes.latency"
	ProbeHeaders    ID = "probes.http.custom-headers"

	// Control plane and cluster resource signals.
	ControlPlanePods        ID = "control-plane.pods"
	APIServerHealth         ID = "control-plane.api.health"
	APIServerLatency        ID = "control-plane.api.latency"
	SchedulerHealth         ID = "control-plane.scheduler"
	ControllerManagerHealth ID = "control-plane.controller-manager"
	EtcdHealth              ID = "control-plane.etcd"
	InformerHealth          ID = "control-plane.informer"
	GenericStatus           ID = "cluster-resources.status"
	CRDDiscovery            ID = "cluster-resources.crd-discovery"

	// Security and admission.
	EventFailureSecurity ID = "security.events.failures"
	RBACAudit            ID = "security.rbac.audit"
	AdmissionFailures    ID = "security.admission.failures"
	TLSMonitoring        ID = "security.tls"
	AuditLog             ID = "security.audit-log"

	// Delivery.
	SingleDestination ID = "delivery.single-destination"
	MultiDestination  ID = "delivery.multi-destination"
	AdvancedRouting   ID = "delivery.routing.advanced"
	Escalation        ID = "delivery.escalation"
	CustomTemplates   ID = "delivery.templates"
	Runbooks          ID = "delivery.runbooks"

	// Reserved premium capabilities. Keeping these IDs in the same catalog
	// makes the binary ready for a future license resolver without scattering
	// product checks throughout monitors.
	PremiumActiveProbes ID = "pro.active-probes"
	PremiumRCAFeedback  ID = "pro.rca-feedback"
	PremiumExports      ID = "pro.incident-export"
)
