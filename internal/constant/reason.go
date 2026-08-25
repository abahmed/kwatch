package constant

// Incident and event reasons form the shared vocabulary used across the
// signal emission sites (handler, resource, pvc), the correlation grouping
// logic, message labels, and enrichment hints. Keeping them as constants
// guarantees the emitted string always matches the string that is grouped,
// labeled, and looked up elsewhere in the codebase.

const (
	// Container lifecycle and container status reasons.
	ReasonStarted              = "Started"
	ReasonKilled               = "Killed"
	ReasonKilling              = "Killing"
	ReasonScheduled            = "Scheduled"
	ReasonPulled               = "Pulled"
	ReasonCompleted            = "Completed"
	ReasonBackOff              = "BackOff"
	ReasonError                = "Error"
	ReasonErrImagePull         = "ErrImagePull"
	ReasonImagePullBackOff     = "ImagePullBackOff"
	ReasonImageInspectError    = "ImageInspectError"
	ReasonInvalidImageName     = "InvalidImageName"
	ReasonCrashLoopBackOff     = "CrashLoopBackOff"
	ReasonCrashLoopHighFreq    = "CrashLoopHighFrequency"
	ReasonOOMKilled            = "OOMKilled"
	ReasonOOMKILLED            = "OOMKILLED"
	ReasonOOMRepeating         = "OOMRepeating"
	ReasonHighRestartCount     = "HighRestartCount"
	ReasonContainerCreating    = "ContainerCreating"
	ReasonPodInitializing      = "PodInitializing"
	ReasonPodCompleted         = "PodCompleted"
	ReasonContainerStatusKnown = "ContainerStatusUnknown"
	ReasonContainersNotReady   = "ContainersNotReady"
	ReasonContainerCannotRun   = "ContainerCannotRun"
	ReasonCreateContainerError = "CreateContainerError"
	ReasonCreateConfigError    = "CreateContainerConfigError"
	ReasonInitContainerError   = "InitContainerError"
	ReasonSandboxError         = "SandboxError"
	ReasonDeadlineExceeded     = "DeadlineExceeded"
	ReasonPostStartHookError   = "PostStartHookError"
	ReasonPreStopHookError     = "PreStopHookError"
	ReasonProbeError           = "ProbeError"
	ReasonStartupProbeFailed   = "StartupProbeFailed"
	ReasonLivenessProbeFailed  = "LivenessProbeFailed"
	ReasonReadinessProbeFailed = "ReadinessProbeFailed"

	// Scheduling and placement reasons.
	ReasonNodeAffinity        = "NodeAffinity"
	ReasonUnschedulable       = "Unschedulable"
	ReasonPodPending          = "PodPending"
	ReasonSchedulingGated     = "SchedulingGated"
	ReasonFailedScheduling    = "FailedScheduling"
	ReasonEvicted             = "Evicted"
	ReasonPreempting          = "Preempting"
	ReasonRegistryUnavailable = "RegistryUnavailable"

	// Node reasons.
	ReasonNodeNotReady         = "NodeNotReady"
	ReasonNotReady             = "NotReady"
	ReasonKubeletReady         = "KubeletReady"
	ReasonKubeletNotReady      = "KubeletNotReady"
	ReasonMemoryPressure       = "MemoryPressure"
	ReasonNodeMemoryPressure   = "NodeMemoryPressure"
	ReasonDiskPressure         = "DiskPressure"
	ReasonPIDPressure          = "PIDPressure"
	ReasonNetworkUnavailable   = "NetworkUnavailable"
	ReasonNodeResourceHigh     = "NodeResourceHigh"
	ReasonNodeResourceCritical = "NodeResourceCritical"

	// Workload rollout reasons.
	ReasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"
	ReasonDeploymentUnavailable    = "DeploymentUnavailable"
	ReasonDaemonSetUnavailable     = "DaemonSetUnavailable"
	ReasonStsUnavailable           = "StsUnavailable"
	ReasonReplicaSetUpdated        = "ReplicaSetUpdated"
	ReasonTooManyReplicas          = "TooManyReplicas"
	ReasonScalingDisabled          = "ScalingDisabled"
	ReasonFailedGetResourceMetric  = "FailedGetResourceMetric"
	ReasonFailedGetScale           = "FailedGetScale"
	ReasonHPAMaxedOut              = "HPAMaxedOut"
	ReasonHPAScalingError          = "HPAScalingError"
	ReasonJobFailed                = "JobFailed"
	ReasonJobSuspended             = "JobSuspended"
	ReasonCronJobSuspended         = "CronJobSuspended"
	ReasonCronJobNotScheduled      = "CronJobNotScheduled"
	ReasonPdbViolation             = "PdbViolation"

	// Service and routing reasons.
	ReasonServiceNoEndpoints       = "ServiceNoEndpoints"
	ReasonIngressBackendNotFound   = "IngressBackendNotFound"
	ReasonWebhookBackendNotFound   = "WebhookBackendNotFound"
	ReasonRestrictiveNetworkPolicy = "RestrictiveNetworkPolicy"

	// Certificates and control plane reasons.
	ReasonTLSCertExpired               = "TLSCertExpired"
	ReasonTLSCertExpiringSoon          = "TLSCertExpiringSoon"
	ReasonControlPlaneComponentFailure = "ControlPlaneComponentFailure"

	// Storage reasons.
	ReasonVolumeUsageHigh = "VolumeUsageHigh"

	// Synthetic and startup reasons.
	ReasonPreExistingAtStartup = "PreExistingAtStartup"
	ReasonNotify               = "notify"
	ReasonTestAlert            = "TestAlert"
)
