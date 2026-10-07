package detection

// Mode constants name every failure mode of modeByReason, grouped like
// its tables. The list was generated from the table values; a new mode
// gets a constant here before a table, row or rule can use it.

// Container lifecycle and container status modes.
const (
	ModeKilled               Mode = "Killed"
	ModeCompleted            Mode = "Completed"
	ModeError                Mode = "Error"
	ModeImagePull            Mode = "ImagePull"
	ModeImagePullInspect     Mode = "ImagePull.Inspect"
	ModeImagePullInvalidName Mode = "ImagePull.InvalidName"
	ModeCrashLoop            Mode = "CrashLoop"
	ModeOOMKilled            Mode = "OOMKilled"
	ModeRestarting           Mode = "Restarting"
	ModeCreating             Mode = "Creating"
	ModeInitializing         Mode = "Initializing"
	ModeNotReady             Mode = "NotReady"
	ModeNeverReady           Mode = "NotReady.Never"
	ModeCannotRun            Mode = "CannotRun"
	ModeCreateError          Mode = "CreateError"
	ModeCreateErrorConfig    Mode = "CreateError.Config"
	ModeInitError            Mode = "InitError"
	ModeInitWaiting          Mode = "InitWaiting"
	ModeDeadlineExceeded     Mode = "DeadlineExceeded"
	ModeHookPostStart        Mode = "Hook.PostStart"
	ModeProbe                Mode = "Probe"
	ModeProbeStartup         Mode = "Probe.Startup"
)

// Scheduling and pod phase modes.
const (
	ModeUnschedulable     Mode = "Unschedulable"
	ModePending           Mode = "Pending"
	ModeFailed            Mode = "Failed"
	ModeStatusUnknown     Mode = "StatusUnknown"
	ModeStuckDeleting     Mode = "StuckDeleting"
	ModeSchedulingGated   Mode = "SchedulingGated"
	ModeEvicted           Mode = "Evicted"
	ModeImagePullRegistry Mode = "ImagePull.Registry"
)

// Node condition and resource usage modes.
const (
	ModeDraining             Mode = "Draining"
	ModeMemoryPressure       Mode = "MemoryPressure"
	ModeDiskPressure         Mode = "DiskPressure"
	ModePIDPressure          Mode = "PIDPressure"
	ModeNetworkUnavailable   Mode = "NetworkUnavailable"
	ModeResourceHigh         Mode = "ResourceHigh"
	ModeResourceCritical     Mode = "ResourceCritical"
	ModeFilesystemUsage      Mode = "Filesystem.Usage"
	ModeInodesUsage          Mode = "Inodes.Usage"
	ModeMemoryHigh           Mode = "MemoryHigh"
	ModeCPUHigh              Mode = "CPUHigh"
	ModeEphemeralStorageHigh Mode = "EphemeralStorageHigh"
	ModeActiveProbe          Mode = "ActiveProbe"
	ModeCPUThrottled         Mode = "CPUThrottled"
	ModePressureStall        Mode = "PressureStall"
	ModeNetworkErrors        Mode = "NetworkErrors"
	ModeRuntimeErrors        Mode = "RuntimeErrors"
)

// Workload, autoscaler, job and budget modes.
const (
	ModeRolloutStuck           Mode = "RolloutStuck"
	ModeUnavailable            Mode = "Unavailable"
	ModeReplicaFailure         Mode = "ReplicaFailure"
	ModeConditionFailure       Mode = "ConditionFailure"
	ModeScalingDisabled        Mode = "Scaling.Disabled"
	ModeScalingNoMetrics       Mode = "Scaling.NoMetrics"
	ModeScalingUpdateFailed    Mode = "Scaling.UpdateFailed"
	ModeScalingInvalidSelector Mode = "Scaling.InvalidSelector"
	ModeScalingMaxedOut        Mode = "Scaling.MaxedOut"
	ModeScalingError           Mode = "Scaling.Error"
	ModeScalingTargetMissing   Mode = "Scaling.TargetMissing"
	ModeJobFailed              Mode = "JobFailed"
	ModeJobFailedDeadline      Mode = "JobFailed.Deadline"
	ModeJobFailedBackoffLimit  Mode = "JobFailed.BackoffLimit"
	ModeSuspended              Mode = "Suspended"
	ModeNotScheduling          Mode = "NotScheduling"
	ModeInvalidSchedule        Mode = "InvalidSchedule"
	ModeDisruptionBudget       Mode = "DisruptionBudget"
	ModeImageDrift             Mode = "ImageDrift"
	ModeReleaseRegression      Mode = "ReleaseRegression"
	ModeLeaseStale             Mode = "LeaseStale"
	ModeDeprecatedAPI          Mode = "DeprecatedAPI"
	ModeAPIServerErrors        Mode = "APIServer.Errors"
	ModePLEGSlow               Mode = "PLEGSlow"
	ModeEvicting               Mode = "Evicting"
	ModeKubeletUnreachable     Mode = "Kubelet.Unreachable"
	ModeKwatchNetwork          Mode = "Kwatch.NetworkRestricted"
	ModeJobRunningLong         Mode = "JobRunningLong"
	ModeScaledToZero           Mode = "ScaledToZero"
	ModeUnused                 Mode = "Unused"
	// ModeDNSServfail is a finer mode of explain's "Resolution": the
	// cluster DNS answers, but with failures.
	ModeDNSServfail Mode = "Resolution.Servfail"
	// Configuration risks carry their reason as their mode; no
	// propagation row matches them.
	ModeRisk                 Mode = "Risk"
	ModeRiskNoReadinessProbe Mode = "Risk.NoReadinessProbe"
	ModeRiskNoMemoryLimit    Mode = "Risk.NoMemoryLimit"
	ModeRiskMutableImageTag  Mode = "Risk.MutableImageTag"
	ModeRiskSingleReplica    Mode = "Risk.SingleReplica"
)

// Service, admission and reference modes.
const (
	ModeNoEndpoints           Mode = "NoEndpoints"
	ModeBackendsDegraded      Mode = "BackendsDegraded"
	ModeLoadBalancerPending   Mode = "LoadBalancerPending"
	ModeBackendMissing        Mode = "BackendMissing"
	ModePortMismatch          Mode = "PortMismatch"
	ModeWebhookBackendMissing Mode = "Webhook.BackendMissing"
	ModeWebhookNoEndpoints    Mode = "Webhook.NoEndpoints"
	ModeWebhookSlow           Mode = "Webhook.Slow"
	ModeWebhookRejecting      Mode = "Webhook.Rejecting"
	ModeWebhookFailingOpen    Mode = "Admission.FailingOpen"
	ModeInvalidPolicy         Mode = "InvalidPolicy"
	ModeInvalidPolicyBinding  Mode = "InvalidPolicy.Binding"
	ModeInvalidPolicyMutating Mode = "InvalidPolicy.Mutating"
	ModeCSRFailed             Mode = "CSRFailed"
	ModeFlowControl           Mode = "FlowControl"
	ModeClaimFailed           Mode = "ClaimFailed"
	ModeInvalidPolicySecurity Mode = "InvalidPolicy.Security"
	ModeMissingServiceAccount Mode = "Missing.ServiceAccount"
	ModeMissingSecret         Mode = "Missing.Secret"
	ModeMissingConfigMap      Mode = "Missing.ConfigMap"
	ModeAPIServiceUnavailable Mode = "APIServiceUnavailable"
	ModeNotReconciling        Mode = "NotReconciling"
	ModeNetworkPolicy         Mode = "NetworkPolicy"
)

// Control-plane, certificate and watch modes.
const (
	ModeCertExpired                  Mode = "Cert.Expired"
	ModeCertExpiring                 Mode = "Cert.Expiring"
	ModeUnavailableAPIServer         Mode = "Unavailable.APIServer"
	ModeLatencyAPIServer             Mode = "Latency.APIServer"
	ModeLatencyWrites                Mode = "Latency.APIServer.Writes"
	ModeLatencyReads                 Mode = "Latency.APIServer.Reads"
	ModeThrottlingAPIServer          Mode = "APIServer.Throttling"
	ModeEtcdLarge                    Mode = "Etcd.Large"
	ModeObjectsHigh                  Mode = "Storage.ObjectsHigh"
	ModeUnavailableScheduler         Mode = "Unavailable.Scheduler"
	ModeUnavailableControllerManager Mode = "Unavailable.ControllerManager"
	ModeUnavailableEtcd              Mode = "Unavailable.Etcd"
	ModeUnavailableCoreDNS           Mode = "Unavailable.CoreDNS"
	ModeActiveProbeLatency           Mode = "ActiveProbe.Latency"
	ModeClusterVersionSkew           Mode = "Cluster.VersionSkew"
	ModeAttachFailed                 Mode = "AttachFailed"
	ModeSnapshotFailed               Mode = "SnapshotFailed"
)

// Storage, quota and deletion modes.
const (
	ModeVolumeFull Mode = "VolumeFull"
	// ModeVolumeInodes is a claim with inodes left to none, bytes or not;
	// it is a kind of VolumeFull, so every row for a full claim matches.
	ModeVolumeInodes      Mode = "VolumeFull.Inodes"
	ModeVolumeFailed      Mode = "VolumeFailed"
	ModeQuotaExhausted    Mode = "QuotaExhausted"
	ModeInvalidLimitRange Mode = "InvalidLimitRange"
)

// Generic condition modes.
const (
	ModeCondition Mode = "Condition"
)

// Pod, container and node runtime modes.
const (
	ModeProbeLiveness     Mode = "Probe.Liveness"
	ModeCrashLoopLiveness Mode = "CrashLoop.Liveness"
	ModeProbeReadiness    Mode = "Probe.Readiness"
	// ModeReadinessFlapping is a readiness failure that comes and goes.
	ModeReadinessFlapping     Mode = "Probe.Readiness.Flapping"
	ModeAdmissionRejected     Mode = "Admission.Rejected"
	ModeExitNotExecutable     Mode = "Exit.NotExecutable"
	ModeExitCommandNotFound   Mode = "Exit.CommandNotFound"
	ModeExitKilled            Mode = "Exit.Killed"
	ModeExitSegfault          Mode = "Exit.Segfault"
	ModeImagePullNeverPull    Mode = "ImagePull.NeverPull"
	ModeHookPreStart          Mode = "Hook.PreStart"
	ModeDiskEvictionThreshold Mode = "Disk.EvictionThreshold"
	ModeDiskImageGCFailed     Mode = "Disk.ImageGCFailed"
	ModeDiskFreeSpaceFailed   Mode = "Disk.FreeSpaceFailed"
	ModeDiskContainerGCFailed Mode = "Disk.ContainerGCFailed"
	ModeNetworkIPExhausted    Mode = "Network.IPExhausted"
	ModeNetworkCNINotReady    Mode = "Network.CNINotReady"
	ModeHeartbeatStale        Mode = "Heartbeat.Stale"
	ModeResizeInfeasible      Mode = "Resize.Infeasible"
	ModeResizeDeferred        Mode = "Resize.Deferred"
	ModeResizeError           Mode = "Resize.Error"
	ModePreempted             Mode = "Preempted"
	ModeKilledGracePeriod     Mode = "Killed.GracePeriod"
	ModePreemptedRepeatedly   Mode = "Preempted.Repeatedly"
	ModeMemoryOvercommitted   Mode = "MemoryOvercommitted"
	// ModeUnusualEvent is a repeated Warning event kwatch has no
	// detector for; the finding's reason names the event.
	ModeUnusualEvent Mode = "UnusualEvent"
)

// Workload, network, storage and admission resource modes.
const (
	// ModeStatefulSetRolloutStuck is a StatefulSet rollout that
	// stopped; it is not the Deployment ModeRolloutStuck.
	ModeStatefulSetRolloutStuck       Mode = "Rollout.Stuck"
	ModeScheduleMissed                Mode = "Schedule.Missed"
	ModeScheduleBlocked               Mode = "Schedule.Blocked"
	ModeScheduleRepeatedFailure       Mode = "Schedule.RepeatedFailure"
	ModeScheduleLastRunFailed         Mode = "Schedule.LastRunFailed"
	ModeScheduleNoRecentSuccess       Mode = "Schedule.NoRecentSuccess"
	ModeLoadBalancerSyncFailed        Mode = "LoadBalancer.SyncFailed"
	ModeIngressTLSSecretMissing       Mode = "Ingress.TLSSecretMissing"
	ModeIngressClassMissing           Mode = "Ingress.ClassMissing"
	ModeReferencePriorityClassMissing Mode = "Reference.PriorityClassMissing"
	ModeReferenceRuntimeClassMissing  Mode = "Reference.RuntimeClassMissing"
	ModeBudgetSelectsNothing          Mode = "Budget.SelectsNothing"
	ModeBudgetOverlap                 Mode = "Budget.Overlap"
	ModeBudgetSyncFailed              Mode = "Budget.SyncFailed"
	ModeBudgetBlocksDrain             Mode = "Budget.BlocksDrain"
	ModeDeviceUnallocated             Mode = "Device.Unallocated"
	ModeDevicePrepareFailed           Mode = "Device.PrepareFailed"
	ModeVolumeDetachFailed            Mode = "Volume.DetachFailed"
	ModeVolumeAttachWaiting           Mode = "Volume.AttachWaiting"
	ModeVolumeProvisioningFailed      Mode = "Volume.ProvisioningFailed"
	ModeVolumeMapFailed               Mode = "Volume.MapFailed"
	ModeQuotaNearLimit                Mode = "Quota.NearLimit"
	ModeCertificateDenied             Mode = "Certificate.Denied"
	ModeCertificateNotIssued          Mode = "Certificate.NotIssued"
	ModeCRDNotEstablished             Mode = "CRD.NotEstablished"
)
