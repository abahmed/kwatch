package detection

import "github.com/abahmed/kwatch/internal/detection/reasons"

// Health says how well an entity is doing its job. The zero value means
// the finding has not been classified yet; Classify fills it in.
type Health uint8

// Health states, from best to worst. Unknown sits last because it means
// kwatch cannot tell, not that the entity is worse than failing.
const (
	// Healthy means the entity works as intended.
	Healthy Health = iota + 1
	// Degraded means the entity works, but less well than it should, or a
	// problem is building up.
	Degraded
	// Failing means the entity does not do its job.
	Failing
	// Unknown means kwatch cannot tell how the entity is doing.
	Unknown
)

// String returns the lower-case name of the health state.
func (h Health) String() string {
	switch h {
	case Healthy:
		return "healthy"
	case Degraded:
		return "degraded"
	case Failing:
		return "failing"
	case Unknown:
		return "unknown"
	default:
		return "unclassified"
	}
}

// healthBySeverity maps a detector's severity to the entity's health. An
// informational finding (a draining node, a suspended schedule) still
// means the entity is not fully serving, so it is Degraded, not Healthy.
var healthBySeverity = map[Severity]Health{
	Info:     Degraded,
	Warning:  Degraded,
	Critical: Failing,
}

// HealthFor returns the health implied by a severity. An unknown severity
// yields Unknown.
func HealthFor(severity Severity) Health {
	if h, ok := healthBySeverity[severity]; ok {
		return h
	}
	return Unknown
}

// ModeFor returns the short, stable failure identity of a reason. Reasons
// without a shorter name are their own mode.
func ModeFor(reason string) Mode {
	if mode, ok := modeByReason[reason]; ok {
		return mode
	}
	return Mode(reason)
}

// Classify fills in Health and Mode when a detector left them unset. It is
// applied to every finding the registry returns, so a new detector cannot
// forget it, and it is idempotent.
func Classify(f Finding) Finding {
	if f.Health == 0 {
		f.Health = HealthFor(f.Severity)
	}
	if f.Mode == "" {
		f.Mode = ModeFor(f.Reason)
	}
	return f
}

// modeByReason names the failure mode of each reason. Modes group reasons
// that describe one condition (both OOM spellings, the HPA metric event
// reasons) and are what the tracker compares to decide whether a finding
// changed. The values are stable: changing one re-announces incidents.
var modeByReason = mergeModes(
	containerModes, schedulingModes, nodeModes, workloadModes,
	serviceModes, controlPlaneModes, storageModes, genericModes,
	syntheticModes, podNodeModes, clusterResourceModes,
)

var containerModes = map[string]Mode{
	reasons.Completed:            ModeCompleted,
	reasons.Error:                ModeError,
	reasons.ErrImagePull:         ModeImagePull,
	reasons.ImagePullBackOff:     ModeImagePull,
	reasons.ImageInspectError:    ModeImagePullInspect,
	reasons.InvalidImageName:     ModeImagePullInvalidName,
	reasons.CrashLoopBackOff:     ModeCrashLoop,
	reasons.OOMKilled:            ModeOOMKilled,
	reasons.Killed:               ModeKilled,
	reasons.OOMKILLED:            ModeOOMKilled,
	reasons.HighRestartCount:     ModeRestarting,
	reasons.ContainerCreating:    ModeCreating,
	reasons.PodInitializing:      ModeInitializing,
	reasons.PodCompleted:         ModeCompleted,
	reasons.ContainersNotReady:   ModeNotReady,
	reasons.ContainerCannotRun:   ModeCannotRun,
	reasons.CreateContainerError: ModeCreateError,
	reasons.CreateConfigError:    ModeCreateErrorConfig,
	reasons.InitContainerError:   ModeInitError,
	reasons.DeadlineExceeded:     ModeDeadlineExceeded,
	reasons.PostStartHookError:   ModeHookPostStart,
	reasons.StartupProbeFailed:   ModeProbeStartup,
}

var schedulingModes = map[string]Mode{
	reasons.Unschedulable:       ModeUnschedulable,
	reasons.PodPending:          ModePending,
	reasons.PodFailed:           ModeFailed,
	reasons.PodStatusUnknown:    ModeStatusUnknown,
	reasons.PodStuckTerminating: ModeStuckDeleting,
	reasons.SchedulingGated:     ModeSchedulingGated,
	reasons.FailedScheduling:    ModeUnschedulable,
	reasons.Evicted:             ModeEvicted,
	reasons.RegistryUnavailable: ModeImagePullRegistry,
}

var nodeModes = map[string]Mode{
	reasons.NodeNotReady:         ModeNotReady,
	reasons.NodeDraining:         ModeDraining,
	reasons.NotReady:             ModeNotReady,
	reasons.MemoryPressure:       ModeMemoryPressure,
	reasons.NodeMemoryPressure:   ModeMemoryPressure,
	reasons.DiskPressure:         ModeDiskPressure,
	reasons.PIDPressure:          ModePIDPressure,
	reasons.NetworkUnavailable:   ModeNetworkUnavailable,
	reasons.NodeResourceHigh:     ModeResourceHigh,
	reasons.NodeResourceCritical: ModeResourceCritical,
	// One mode per resource: severity says how full it is, and the
	// Critical reasons, which only persisted findings carry, share it.
	reasons.NodeFilesystemHigh:            ModeFilesystemUsage,
	reasons.NodeFilesystemCritical:        ModeFilesystemUsage,
	reasons.NodeInodesHigh:                ModeInodesUsage,
	reasons.NodeInodesCritical:            ModeInodesUsage,
	reasons.ContainerMemoryHigh:           ModeMemoryHigh,
	reasons.ContainerCPUHigh:              ModeCPUHigh,
	reasons.ContainerEphemeralStorageHigh: ModeEphemeralStorageHigh,
	reasons.ActiveProbeFailure:            ModeActiveProbe,
	reasons.ContainerCPUThrottled:         ModeCPUThrottled,
	reasons.NodePSIHigh:                   ModePressureStall,
	reasons.NodeNetworkErrors:             ModeNetworkErrors,
	reasons.NodeRuntimeErrors:             ModeRuntimeErrors,
	reasons.NodeMemoryOvercommitted:       ModeMemoryOvercommitted,
}

var workloadModes = map[string]Mode{
	reasons.ProgressDeadlineExceeded:     ModeRolloutStuck,
	reasons.DeploymentUnavailable:        ModeUnavailable,
	reasons.DeploymentProgressing:        ModeRolloutStuck,
	reasons.DeploymentAvailable:          ModeUnavailable,
	reasons.DeploymentReplicaFailure:     ModeReplicaFailure,
	reasons.ReplicaSetFailure:            ModeReplicaFailure,
	reasons.DaemonSetUnavailable:         ModeUnavailable,
	reasons.StsUnavailable:               ModeUnavailable,
	reasons.ScalingDisabled:              ModeScalingDisabled,
	reasons.FailedGetResourceMetric:      ModeScalingNoMetrics,
	reasons.FailedComputeMetricsReplicas: ModeScalingNoMetrics,
	reasons.FailedGetMetrics:             ModeScalingNoMetrics,
	reasons.FailedGetScale:               ModeScalingTargetMissing,
	reasons.FailedUpdateScale:            ModeScalingUpdateFailed,
	reasons.HPAInvalidSelector:           ModeScalingInvalidSelector,
	reasons.HPAMaxedOut:                  ModeScalingMaxedOut,
	reasons.StatefulSetCondition:         ModeConditionFailure,
	reasons.DaemonSetCondition:           ModeConditionFailure,
	reasons.HPAScalingError:              ModeScalingError,
	reasons.HPATargetMissing:             ModeScalingTargetMissing,
	reasons.WorkloadNeverReady:           ModeNeverReady,
	reasons.KubeletUnreachable:           ModeKubeletUnreachable,
	reasons.KwatchNetworkRestricted:      ModeKwatchNetwork,
	reasons.JobRunningLong:               ModeJobRunningLong,
	reasons.ScaledToZeroRouted:           ModeScaledToZero,
	reasons.JobFailed:                    ModeJobFailed,
	reasons.JobDeadlineExceeded:          ModeJobFailedDeadline,
	reasons.JobBackoffLimitExceeded:      ModeJobFailedBackoffLimit,
	reasons.JobSuspended:                 ModeSuspended,
	reasons.CronJobSuspended:             ModeSuspended,
	reasons.CronJobNotScheduled:          ModeNotScheduling,
	reasons.CronJobInvalidSchedule:       ModeInvalidSchedule,
	reasons.PdbViolation:                 ModeDisruptionBudget,
	reasons.ImageDigestDrift:             ModeImageDrift,
	reasons.ReleaseRegression:            ModeReleaseRegression,
	reasons.LeaseStale:                   ModeLeaseStale,
	reasons.DeprecatedAPIInUse:           ModeDeprecatedAPI,
	reasons.APIServerErrors:              ModeAPIServerErrors,
	reasons.CoreDNSServfail:              ModeDNSServfail,
	reasons.NodePLEGSlow:                 ModePLEGSlow,
	reasons.NodeEvicting:                 ModeEvicting,
	reasons.ServiceUnused:                ModeUnused,
	reasons.ClaimUnused:                  ModeUnused,
	reasons.RiskPrefix:                   ModeRisk,
	reasons.RiskNoReadinessProbe:         ModeRiskNoReadinessProbe,
	reasons.RiskNoMemoryLimit:            ModeRiskNoMemoryLimit,
	reasons.RiskMutableImageTag:          ModeRiskMutableImageTag,
	reasons.RiskSingleReplica:            ModeRiskSingleReplica,
	reasons.RiskSingleNode:               ModeRiskSingleNode,
	reasons.RiskPrivileged:               ModeRiskPrivileged,
}

var serviceModes = map[string]Mode{
	reasons.ServiceNoEndpoints:               ModeNoEndpoints,
	reasons.ServiceBackendsDegraded:          ModeBackendsDegraded,
	reasons.LoadBalancerPending:              ModeLoadBalancerPending,
	reasons.IngressBackendNotFound:           ModeBackendMissing,
	reasons.WebhookBackendNotFound:           ModeWebhookBackendMissing,
	reasons.WebhookNoEndpoints:               ModeWebhookNoEndpoints,
	reasons.WebhookSlow:                      ModeWebhookSlow,
	reasons.WebhookRejecting:                 ModeWebhookRejecting,
	reasons.WebhookFailingOpen:               ModeWebhookFailingOpen,
	reasons.AdmissionPolicyInvalid:           ModeInvalidPolicy,
	reasons.AdmissionBindingInvalid:          ModeInvalidPolicyBinding,
	reasons.MutatingAdmissionPolicyInvalid:   ModeInvalidPolicyMutating,
	reasons.CertificateSigningRequestFailure: ModeCSRFailed,
	reasons.APIPriorityAndFairnessFailure:    ModeFlowControl,
	reasons.ResourceClaimFailure:             ModeClaimFailed,
	reasons.PodSecurityPolicyInvalid:         ModeInvalidPolicySecurity,
	reasons.ServiceAccountMissing:            ModeMissingServiceAccount,
	reasons.ProjectedSecretMissing:           ModeMissingSecret,
	reasons.ProjectedConfigMapMissing:        ModeMissingConfigMap,
	reasons.APIServiceFailure:                ModeAPIServiceUnavailable,
	reasons.CustomResourceFailure:            ModeNotReconciling,
	reasons.RestrictiveNetworkPolicy:         ModeNetworkPolicy,
}

var controlPlaneModes = map[string]Mode{
	reasons.TLSCertExpired:               ModeCertExpired,
	reasons.TLSCertExpiringSoon:          ModeCertExpiring,
	reasons.APIServerUnavailable:         ModeUnavailableAPIServer,
	reasons.APIServerLatency:             ModeLatencyAPIServer,
	reasons.APIServerWritesSlow:          ModeLatencyWrites,
	reasons.APIServerReadsSlow:           ModeLatencyReads,
	reasons.APIServerThrottling:          ModeThrottlingAPIServer,
	reasons.EtcdDatabaseLarge:            ModeEtcdLarge,
	reasons.StorageObjectsHigh:           ModeObjectsHigh,
	reasons.SchedulerUnavailable:         ModeUnavailableScheduler,
	reasons.ControllerManagerUnavailable: ModeUnavailableControllerManager,
	reasons.EtcdUnavailable:              ModeUnavailableEtcd,
	reasons.CoreDNSUnavailable:           ModeUnavailableCoreDNS,
	reasons.ActiveProbeLatency:           ModeActiveProbeLatency,
	reasons.ClusterVersionSkew:           ModeClusterVersionSkew,
	reasons.VolumeAttachmentFailure:      ModeAttachFailed,
	reasons.VolumeSnapshotFailure:        ModeSnapshotFailed,
}

var storageModes = map[string]Mode{
	reasons.VolumeUsageHigh:        ModeVolumeFull,
	reasons.VolumeFillingUp:        ModeVolumeFillingUp,
	reasons.PersistentVolumeClaim:  ModeClaimFailed,
	reasons.PersistentVolume:       ModeVolumeFailed,
	reasons.ResourceQuotaExhausted: ModeQuotaExhausted,
	reasons.LimitRangeInvalid:      ModeInvalidLimitRange,
	reasons.NamespaceStuck:         ModeStuckDeleting,
	reasons.NodeStuckTerminating:   ModeStuckDeleting,
}

var genericModes = map[string]Mode{
	// UnusualEventPrefix is a prefix: the finding sets the mode itself.
	reasons.UnusualEventPrefix: ModeUnusualEvent,
	reasons.ConditionFailure:   ModeCondition,
	reasons.GenerationLagging:  ModeNotReconciling,
	reasons.StuckDeleting:      ModeStuckDeleting,
	reasons.PhaseFailed:        ModeFailed,
	reasons.PhasePending:       ModePending,
}

var syntheticModes = map[string]Mode{}

func mergeModes(groups ...map[string]Mode) map[string]Mode {
	out := make(map[string]Mode)
	for _, group := range groups {
		for reason, mode := range group {
			out[reason] = mode
		}
	}
	return out
}

// Pod, container and node failure modes (pod runtime).
var podNodeModes = map[string]Mode{
	reasons.LivenessProbeFailed:    ModeProbeLiveness,
	reasons.LivenessKilled:         ModeCrashLoopLiveness,
	reasons.ReadinessProbeFailed:   ModeProbeReadiness,
	reasons.PodAdmissionRejected:   ModeAdmissionRejected,
	reasons.ExitNotExecutable:      ModeExitNotExecutable,
	reasons.ExitCommandNotFound:    ModeExitCommandNotFound,
	reasons.ExitKilled:             ModeExitKilled,
	reasons.ExitSegfault:           ModeExitSegfault,
	reasons.ErrImageNeverPull:      ModeImagePullNeverPull,
	reasons.PreStartHookError:      ModeHookPreStart,
	reasons.EvictionThresholdMet:   ModeDiskEvictionThreshold,
	reasons.ImageGCFailed:          ModeDiskImageGCFailed,
	reasons.FreeDiskSpaceFailed:    ModeDiskFreeSpaceFailed,
	reasons.ContainerGCFailed:      ModeDiskContainerGCFailed,
	reasons.NodePodIPExhausted:     ModeNetworkIPExhausted,
	reasons.NodeCNINotReady:        ModeNetworkCNINotReady,
	reasons.NodeHeartbeatStale:     ModeHeartbeatStale,
	reasons.PodResizeInfeasible:    ModeResizeInfeasible,
	reasons.PodResizeDeferred:      ModeResizeDeferred,
	reasons.PodResizeError:         ModeResizeError,
	reasons.PodPreemptedRepeatedly: ModePreemptedRepeatedly,
}

// Workload, network, storage and admission failure modes.
var clusterResourceModes = map[string]Mode{
	reasons.StatefulSetRolloutStuck:       ModeStatefulSetRolloutStuck,
	reasons.CronJobMissedRuns:             ModeScheduleMissed,
	reasons.CronJobBlocked:                ModeScheduleBlocked,
	reasons.CronJobRepeatedFailure:        ModeScheduleRepeatedFailure,
	reasons.CronJobLastRunFailed:          ModeScheduleLastRunFailed,
	reasons.CronJobNoRecentSuccess:        ModeScheduleNoRecentSuccess,
	reasons.LoadBalancerSyncFailed:        ModeLoadBalancerSyncFailed,
	reasons.IngressTLSSecretMissing:       ModeIngressTLSSecretMissing,
	reasons.IngressClassMissing:           ModeIngressClassMissing,
	reasons.PriorityClassMissing:          ModeReferencePriorityClassMissing,
	reasons.RuntimeClassMissing:           ModeReferenceRuntimeClassMissing,
	reasons.PdbSelectsNothing:             ModeBudgetSelectsNothing,
	reasons.PdbOverlap:                    ModeBudgetOverlap,
	reasons.PdbSyncFailed:                 ModeBudgetSyncFailed,
	reasons.PdbBlocksDrain:                ModeBudgetBlocksDrain,
	reasons.ResourceClaimUnallocated:      ModeDeviceUnallocated,
	reasons.FailedPrepareDynamicResources: ModeDevicePrepareFailed,
	reasons.VolumeDetachFailure:           ModeVolumeDetachFailed,
	reasons.VolumeAttachWaiting:           ModeVolumeAttachWaiting,
	reasons.ProvisioningFailed:            ModeVolumeProvisioningFailed,
	reasons.FailedMapVolume:               ModeVolumeMapFailed,
	reasons.ResourceQuotaNearLimit:        ModeQuotaNearLimit,
	reasons.CertificateDenied:             ModeCertificateDenied,
	reasons.CertificateNotIssued:          ModeCertificateNotIssued,
	reasons.CRDNotEstablished:             ModeCRDNotEstablished,
}
