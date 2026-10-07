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
	// InitContainerWaiting is an init container that runs far longer
	// than usual and holds the pod in Init.
	InitContainerWaiting = "InitContainerWaiting"
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
	NodeNotReady       = "NodeNotReady"
	NodeDraining       = "NodeDraining"
	NotReady           = "NotReady"
	MemoryPressure     = "MemoryPressure"
	NodeMemoryPressure = "NodeMemoryPressure"
	DiskPressure       = "DiskPressure"
	PIDPressure        = "PIDPressure"
	NetworkUnavailable = "NetworkUnavailable"
	NodeFilesystemHigh = "NodeFilesystemUsageHigh"
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
	LoadBalancerPending     = "LoadBalancerProvisioning"
	// IngressBackendNotFound is an Ingress or a Gateway API route that
	// sends traffic to a Service that does not exist. The name predates
	// route support and is kept: audit logs and filters refer to it.
	IngressBackendNotFound = "IngressBackendNotFound"
	WebhookBackendNotFound = "WebhookBackendNotFound"
	// ServicePortMismatch is a Service whose targetPort matches no port
	// of the pods it selects. IngressBackendPortMissing is an Ingress
	// rule that names a Service port the Service does not have.
	ServicePortMismatch              = "ServicePortMismatch"
	IngressBackendPortMissing        = "IngressBackendPortMissing"
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
	VolumeInodesHigh       = "VolumeInodesHigh"
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

// Synthetic reasons kwatch derives rather than reads from one object.
const (
	// UnusualEventPrefix starts the reason of a Warning event kwatch has
	// no detector for, repeated enough to matter: "UnusualEvent.SystemOOM".
	UnusualEventPrefix = "UnusualEvent."
	// NodeMemoryOvercommitted is a node whose pods' memory limits add up
	// to far more memory than it has.
	NodeMemoryOvercommitted = "NodeMemoryOvercommitted"
	// ImageDigestDrift is a workload whose pods run different builds of
	// the same image tag.
	ImageDigestDrift = "ImageDigestDrift"
	// ReleaseRegression is a Deployment whose new revision restarts far
	// more often than the revision it replaced did.
	ReleaseRegression = "ReleaseRegression"
	// LeaseStale is a controller's Lease that its running holder stopped
	// renewing: the controller runs but does not work.
	LeaseStale = "LeaseStale"
	// DeprecatedAPIInUse is an API version that something still requests
	// and that a Kubernetes release removes.
	DeprecatedAPIInUse = "DeprecatedAPIInUse"
	// APIServerErrors is the API server answering a notable share of
	// requests with server errors.
	APIServerErrors = "APIServerErrors"
	// APIServerWritesSlow is the API server taking seconds to create,
	// update or delete objects.
	APIServerWritesSlow = "APIServerWritesSlow"
	// APIServerReadsSlow is the API server taking seconds to answer gets
	// and lists.
	APIServerReadsSlow = "APIServerReadsSlow"
	// APIServerThrottling is the API server rejecting requests because a
	// priority level is full.
	APIServerThrottling = "APIServerThrottling"
	// EtcdDatabaseLarge is an etcd database close to its default size
	// quota, past which etcd refuses writes.
	EtcdDatabaseLarge = "EtcdDatabaseLarge"
	// StorageObjectsHigh is one resource with so many stored objects
	// that listing it strains the API server and etcd.
	StorageObjectsHigh = "StorageObjectsHigh"
	// WebhookSlow is an admission webhook that takes seconds per call,
	// slowing every request it intercepts.
	WebhookSlow = "WebhookSlow"
	// WebhookRejecting is an admission webhook that fails a large share
	// of its calls while set to fail closed: those requests are refused.
	WebhookRejecting = "WebhookRejecting"
	// WebhookFailingOpen is an admission webhook that fails a large share
	// of its calls but is set to ignore failures: its checks are skipped.
	WebhookFailingOpen = "WebhookFailingOpen"
	// CoreDNSServfail is the cluster DNS failing a notable share of
	// lookups with SERVFAIL.
	CoreDNSServfail = "CoreDNSServfail"
	// NodePLEGSlow is a kubelet whose pod lifecycle relist takes so
	// long that it falls behind its pods.
	NodePLEGSlow = "NodePLEGSlow"
	// NodeEvicting is a kubelet evicting pods right now.
	NodeEvicting = "NodeEvicting"
	// ServiceUnused is a Service that has selected no pod for a day.
	ServiceUnused = "ServiceUnused"
	// ClaimUnused is a bound claim no pod has mounted for a day.
	ClaimUnused = "ClaimUnused"
)

// Configuration risks: advisory findings about how a workload is set
// up. They are never announced on their own; a failure quotes one as a
// consequence when it shows what the risk cost. They share RiskPrefix.
const (
	RiskPrefix           = "Risk."
	RiskNoReadinessProbe = "Risk.NoReadinessProbe"
	RiskNoMemoryLimit    = "Risk.NoMemoryLimit"
	RiskMutableImageTag  = "Risk.MutableImageTag"
	RiskSingleReplica    = "Risk.SingleReplica"
)

// UnusualEvent is the reason for a repeated Warning event kwatch does
// not otherwise detect, named after the event's own reason.
func UnusualEvent(eventReason string) string {
	return UnusualEventPrefix + eventReason
}

// Pod, container and node failure modes (pod runtime). Kubernetes strings
// matched by the detectors (kubelet admission reasons, event reasons,
// probe messages) are verified against kubernetes/kubernetes master.
const (
	LivenessProbeFailed  = "LivenessProbeFailed"
	LivenessKilled       = "LivenessKilled"
	ReadinessProbeFailed = "ReadinessProbeFailed"
	// ReadinessFlapping is a workload whose pods keep switching between
	// ready and not ready, so the endpoints of its Service keep changing.
	ReadinessFlapping      = "ReadinessFlapping"
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
	PodPreempted           = "PodPreempted"
	PodKilledAtGrace       = "PodKilledAtGrace"
	ClusterVersionSkew     = "ClusterVersionSkew"
)

// Workload, network, storage and admission failure modes.
const (
	StatefulSetRolloutStuck = "StatefulSetRolloutStuck"
	CronJobMissedRuns       = "CronJobMissedRuns"
	CronJobBlocked          = "CronJobBlocked"
	CronJobRepeatedFailure  = "CronJobRepeatedFailure"
	// CronJobLastRunFailed is a CronJob whose latest run failed and has
	// not been followed by a success; it waits for the digest.
	CronJobLastRunFailed = "CronJobLastRunFailed"
	// CronJobNoRecentSuccess is a CronJob that has not succeeded for
	// several scheduled runs; it waits for the digest.
	CronJobNoRecentSuccess        = "CronJobNoRecentSuccess"
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

// Reasons for workloads that run but never serve, autoscalers that point
// at nothing, and kubelets kwatch cannot read.
const (
	// WorkloadNeverReady is a workload whose running pods are not ready
	// and have not crashed: a readiness probe that never passes.
	WorkloadNeverReady = "WorkloadNeverReady"
	// HPATargetMissing is an autoscaler whose scale target does not
	// exist.
	HPATargetMissing = "HPATargetMissing"
	// KubeletUnreachable is a Ready node whose kubelet kwatch could not
	// reach at least three times within six hours.
	KubeletUnreachable = "KubeletUnreachable"
	// JobRunningLong is a CronJob's Job that has run far longer than
	// the recent runs of that CronJob took.
	JobRunningLong = "JobRunningLong"
	// ScaledToZeroRouted is a workload scaled to zero replicas that a
	// Service, an Ingress or a Gateway route still sends traffic to.
	ScaledToZeroRouted = "ScaledToZeroRouted"
	// KwatchNetworkRestricted is kwatch failing to reach every one of
	// its probed dependencies in the same round: a sign that its own
	// network is restricted, not that they are all down.
	KwatchNetworkRestricted = "KwatchNetworkRestricted"
)
