package reasons

// Finding reasons form the shared vocabulary of detectors, root-cause
// rules, incident policy and message composition. Keeping them as constants
// guarantees the emitted string always matches the string that is grouped,
// labeled, and looked up elsewhere in the codebase. The values are stable:
// people grep audit logs and route alerts by them.

// Container lifecycle and container status reasons.
const (
	Completed            = "Completed"
	Error                = "Error"
	ErrImagePull         = "ErrImagePull"
	ImagePullBackOff     = "ImagePullBackOff"
	ImageInspectError    = "ImageInspectError"
	InvalidImageName     = "InvalidImageName"
	CrashLoopBackOff     = "CrashLoopBackOff"
	OOMKilled            = "OOMKilled"
	Killed               = "Killed"
	OOMKILLED            = "OOMKILLED"
	HighRestartCount     = "HighRestartCount"
	ContainerCreating    = "ContainerCreating"
	PodInitializing      = "PodInitializing"
	PodCompleted         = "PodCompleted"
	ContainersNotReady   = "ContainersNotReady"
	ContainerCannotRun   = "ContainerCannotRun"
	CreateContainerError = "CreateContainerError"
	CreateConfigError    = "CreateContainerConfigError"
	InitContainerError   = "InitContainerError"
	DeadlineExceeded     = "DeadlineExceeded"
	PostStartHookError   = "PostStartHookError"
	StartupProbeFailed   = "StartupProbeFailed"
)

// Scheduling and placement reasons.
const (
	Unschedulable       = "Unschedulable"
	PodPending          = "PodPending"
	PodFailed           = "PodFailed"
	PodStatusUnknown    = "PodStatusUnknown"
	PodStuckTerminating = "PodStuckTerminating"
	SchedulingGated     = "SchedulingGated"
	FailedScheduling    = "FailedScheduling"
	Evicted             = "Evicted"
	RegistryUnavailable = "RegistryUnavailable"
)

// Node reasons.
const (
	NodeNotReady         = "NodeNotReady"
	NodeDraining         = "NodeDraining"
	NotReady             = "NotReady"
	MemoryPressure       = "MemoryPressure"
	NodeMemoryPressure   = "NodeMemoryPressure"
	DiskPressure         = "DiskPressure"
	PIDPressure          = "PIDPressure"
	NetworkUnavailable   = "NetworkUnavailable"
	NodeResourceHigh     = "NodeResourceHigh"
	NodeResourceCritical = "NodeResourceCritical"
	NodeFilesystemHigh   = "NodeFilesystemUsageHigh"
	// NodeFilesystemCritical and NodeInodesCritical are never raised: the
	// High reasons turn critical instead. They exist so findings and
	// incidents persisted by earlier versions keep their mode.
	NodeFilesystemCritical        = "NodeFilesystemUsageCritical"
	NodeInodesHigh                = "NodeInodesUsageHigh"
	NodeInodesCritical            = "NodeInodesUsageCritical"
	ContainerMemoryHigh           = "ContainerMemoryUsageHigh"
	ContainerCPUHigh              = "ContainerCPUUsageHigh"
	ContainerEphemeralStorageHigh = "ContainerEphemeralStorageUsageHigh"
	ActiveProbeFailure            = "ActiveProbeFailure"
	ContainerCPUThrottled         = "ContainerCPUThrottled"
	NodePSIHigh                   = "NodePressureStall"
	NodeNetworkErrors             = "NodeNetworkErrors"
	NodeRuntimeErrors             = "NodeRuntimeErrors"
)

// Workload rollout reasons.
const (
	ProgressDeadlineExceeded = "ProgressDeadlineExceeded"
	DeploymentUnavailable    = "DeploymentUnavailable"
	DeploymentProgressing    = "DeploymentProgressingFalse"
	DeploymentAvailable      = "DeploymentAvailableFalse"
	DeploymentReplicaFailure = "DeploymentReplicaFailure"
	ReplicaSetFailure        = "ReplicaSetFailure"
	DaemonSetUnavailable     = "DaemonSetUnavailable"
	StsUnavailable           = "StsUnavailable"
	ScalingDisabled          = "ScalingDisabled"
	FailedGetResourceMetric  = "FailedGetResourceMetric"
	// FailedComputeMetricsReplicas, FailedGetMetrics and FailedGetScale are
	// event reasons the HPA controller reports for one missing-metrics
	// condition; they are folded into FailedGetResourceMetric.
	FailedComputeMetricsReplicas = "FailedComputeMetricsReplicas"
	FailedGetMetrics             = "FailedGetMetrics"
	FailedGetScale               = "FailedGetScale"
	FailedUpdateScale            = "FailedUpdateScale"
	HPAInvalidSelector           = "HPAInvalidSelector"
	HPAMaxedOut                  = "HPAMaxedOut"
	StatefulSetCondition         = "StatefulSetConditionFailure"
	DaemonSetCondition           = "DaemonSetConditionFailure"
	HPAScalingError              = "HPAScalingError"
	JobFailed                    = "JobFailed"
	JobDeadlineExceeded          = "JobDeadlineExceeded"
	JobBackoffLimitExceeded      = "JobBackoffLimitExceeded"
	JobSuspended                 = "JobSuspended"
	CronJobSuspended             = "CronJobSuspended"
	CronJobNotScheduled          = "CronJobNotScheduled"
	CronJobInvalidSchedule       = "CronJobInvalidSchedule"
	PdbViolation                 = "PdbViolation"
)

// Service and routing reasons.
const (
	ServiceNoEndpoints      = "ServiceNoEndpoints"
	ServiceBackendsDegraded = "ServiceBackendsDegraded"
	ServicePortMismatch     = "ServicePortMismatch"
	LoadBalancerPending     = "LoadBalancerProvisioning"
	// IngressBackendNotFound is an Ingress or a Gateway API route that
	// sends traffic to a Service that does not exist. The name predates
	// route support and is kept: audit logs and filters refer to it.
	IngressBackendNotFound           = "IngressBackendNotFound"
	WebhookBackendNotFound           = "WebhookBackendNotFound"
	WebhookNoEndpoints               = "WebhookNoEndpoints"
	AdmissionPolicyInvalid           = "AdmissionPolicyInvalid"
	AdmissionBindingInvalid          = "AdmissionPolicyBindingInvalid"
	MutatingAdmissionPolicyInvalid   = "MutatingAdmissionPolicyInvalid"
	CertificateSigningRequestFailure = "CertificateSigningRequestFailure"
	APIPriorityAndFairnessFailure    = "APIPriorityAndFairnessFailure"
	ResourceClaimFailure             = "ResourceClaimFailure"
	PodSecurityPolicyInvalid         = "PodSecurityPolicyInvalid"
	ServiceAccountMissing            = "ServiceAccountMissing"
	ProjectedSecretMissing           = "ProjectedSecretMissing"
	ProjectedConfigMapMissing        = "ProjectedConfigMapMissing"
	APIServiceFailure                = "APIServiceFailure"
	CustomResourceFailure            = "CustomResourceFailure"
	RestrictiveNetworkPolicy         = "RestrictiveNetworkPolicy"
)

// Certificates and control plane reasons.
const (
	TLSCertExpired               = "TLSCertExpired"
	TLSCertExpiringSoon          = "TLSCertExpiringSoon"
	APIServerUnavailable         = "APIServerUnavailable"
	APIServerLatency             = "APIServerLatency"
	SchedulerUnavailable         = "SchedulerUnavailable"
	ControllerManagerUnavailable = "ControllerManagerUnavailable"
	EtcdUnavailable              = "EtcdUnavailable"
	CoreDNSUnavailable           = "CoreDNSUnavailable"
	ActiveProbeLatency           = "ActiveProbeLatency"
	VolumeAttachmentFailure      = "VolumeAttachmentFailure"
	VolumeSnapshotFailure        = "VolumeSnapshotFailure"
)

// Storage reasons.
const (
	VolumeUsageHigh        = "VolumeUsageHigh"
	VolumeFillingUp        = "VolumeFillingUp"
	PersistentVolumeClaim  = "PersistentVolumeClaimFailure"
	PersistentVolume       = "PersistentVolumeFailure"
	ResourceQuotaExhausted = "ResourceQuotaExhausted"
	LimitRangeInvalid      = "LimitRangeInvalid"
	NamespaceStuck         = "NamespaceStuckTerminating"
	NodeStuckTerminating   = "NodeStuckTerminating"
)

// Generic health reasons, derived from the fields every object carries
// whatever its kind. A failing condition's reason is Condition(type).
const (
	ConditionFailure  = "ConditionFailure"
	GenerationLagging = "GenerationLagging"
	StuckDeleting     = "StuckDeleting"
	PhaseFailed       = "PhaseFailed"
	PhasePending      = "PhasePending"
)

// Condition returns the reason of a failing status condition, such as
// "ConditionFailure.Ready".
func Condition(conditionType string) string {
	return ConditionFailure + "." + conditionType
}

// Synthetic and startup reasons.
const ()

// Pod, container and node failure modes (pod runtime). Kubernetes strings
// matched by the detectors (kubelet admission reasons, event reasons,
// probe messages) are verified against kubernetes/kubernetes master.
const (
	LivenessProbeFailed    = "LivenessProbeFailed"
	ReadinessProbeFailed   = "ReadinessProbeFailed"
	PodAdmissionRejected   = "PodAdmissionRejected"
	ExitNotExecutable      = "ContainerExitNotExecutable"
	ExitCommandNotFound    = "ContainerExitCommandNotFound"
	ExitKilled             = "ContainerExitKilled"
	ExitSegfault           = "ContainerExitSegfault"
	ErrImageNeverPull      = "ErrImageNeverPull"
	PreStartHookError      = "PreStartHookError"
	EvictionThresholdMet   = "EvictionThresholdMet"
	ImageGCFailed          = "ImageGCFailed"
	FreeDiskSpaceFailed    = "FreeDiskSpaceFailed"
	ContainerGCFailed      = "ContainerGCFailed"
	NodePodIPExhausted     = "NodePodIPExhausted"
	NodeCNINotReady        = "NodeCNINotReady"
	NodeHeartbeatStale     = "NodeHeartbeatStale"
	PodResizeInfeasible    = "PodResizeInfeasible"
	PodResizeDeferred      = "PodResizeDeferred"
	PodResizeError         = "PodResizeError"
	PodPreemptedRepeatedly = "PodPreemptedRepeatedly"
	ClusterVersionSkew     = "ClusterVersionSkew"
)

// Workload, network, storage and admission failure modes.
const (
	StatefulSetRolloutStuck       = "StatefulSetRolloutStuck"
	CronJobMissedRuns             = "CronJobMissedRuns"
	CronJobBlocked                = "CronJobBlocked"
	CronJobRepeatedFailure        = "CronJobRepeatedFailure"
	LoadBalancerSyncFailed        = "LoadBalancerSyncFailed"
	IngressTLSSecretMissing       = "IngressTLSSecretMissing"
	IngressClassMissing           = "IngressClassMissing"
	PriorityClassMissing          = "PriorityClassMissing"
	RuntimeClassMissing           = "RuntimeClassMissing"
	PdbSelectsNothing             = "PdbSelectsNothing"
	PdbOverlap                    = "PdbOverlap"
	PdbSyncFailed                 = "PdbSyncFailed"
	PdbBlocksDrain                = "PdbBlocksDrain"
	ResourceClaimUnallocated      = "ResourceClaimUnallocated"
	FailedPrepareDynamicResources = "FailedPrepareDynamicResources"
	VolumeDetachFailure           = "VolumeDetachFailure"
	VolumeAttachWaiting           = "VolumeAttachWaiting"
	ProvisioningFailed            = "ProvisioningFailed"
	FailedMapVolume               = "FailedMapVolume"
	ResourceQuotaNearLimit        = "ResourceQuotaNearLimit"
	CertificateDenied             = "CertificateSigningRequestDenied"
	CertificateNotIssued          = "CertificateSigningRequestNotIssued"
	CRDNotEstablished             = "CustomResourceDefinitionNotEstablished"
)
