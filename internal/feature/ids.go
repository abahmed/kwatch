// Package feature contains the product capability catalog used by kwatch.sh.
package feature

// ID is a stable capability identifier. Once released, an ID must not be
// silently renamed.
type ID string

const (
	PodDetection         ID = "core.detection.pods"
	PodScheduling        ID = "core.pods.scheduling"
	PodOOM               ID = "core.pods.oom"
	PodReadiness         ID = "core.pods.readiness"
	PodRestarts          ID = "core.pods.restarts"
	WorkloadDetection    ID = "core.detection.workloads"
	WorkloadRollouts     ID = "core.workloads.rollouts"
	JobFailures          ID = "core.workloads.jobs"
	DisruptionBudgets    ID = "core.workloads.pdb"
	Autoscaling          ID = "core.workloads.hpa"
	NodeDetection        ID = "core.detection.nodes"
	NodeUsage            ID = "core.nodes.usage"
	StorageDetection     ID = "core.detection.storage"
	VolumeUsage          ID = "core.storage.usage"
	NetworkDetection     ID = "core.detection.network"
	AdmissionDetection   ID = "core.detection.admission"
	CertificateDetection ID = "core.detection.certificates"
	QuotaDetection       ID = "core.detection.quota"
	CustomResources      ID = "core.detection.custom-resources"
	ControlPlane         ID = "core.detection.control-plane"
	ActiveProbes         ID = "core.detection.probes"
	RootCause            ID = "analysis.root-cause"
	ChangeCorrelation    ID = "analysis.changes"
	Impact               ID = "analysis.impact"
	Investigation        ID = "analysis.investigation"
	NoiseControl         ID = "problems.noise-control"
	Flapping             ID = "problems.flapping"
	StartupSummary       ID = "problems.startup-summary"
	DiskState            ID = "problems.state"
	DowntimeChanges      ID = "problems.downtime-changes"
	Scope                ID = "policy.scope"
	SeverityOverrides    ID = "policy.severity"
	Maintenance          ID = "policy.maintenance"
	Runbooks             ID = "policy.runbooks"
	ProviderRouting      ID = "delivery.routing"
	NativeThreads        ID = "delivery.threads"
	CustomTemplates      ID = "delivery.templates"
	AuditLog             ID = "delivery.audit-log"
	RBACAudit            ID = "security.rbac-audit"
)
