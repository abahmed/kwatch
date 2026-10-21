package detection

import (
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// workloadReasons lists the workload, autoscaler, job and resource reasons.
var workloadReasons = []ReasonInfo{
	reason(reasons.ProgressDeadlineExceeded, ModeRolloutStuck),
	reason(reasons.DeploymentUnavailable, ModeUnavailable),
	reason(reasons.DeploymentReplicaFailure, ModeReplicaFailure),
	reason(reasons.ReplicaSetFailure, ModeReplicaFailure),
	reason(reasons.DaemonSetUnavailable, ModeUnavailable),
	reason(reasons.DaemonSetMisscheduled, ModeDaemonSetMisscheduled).
		digest(),
	reason(reasons.StsUnavailable, ModeUnavailable),
	reason(reasons.ScalingDisabled, ModeScalingDisabled),
	reason(reasons.FailedGetResourceMetric, ModeScalingNoMetrics).digest().
		shown(kube.KindHPA),
	reason(reasons.FailedComputeMetricsReplicas, ModeScalingNoMetrics).
		shown(kube.KindHPA),
	reason(reasons.FailedGetScale, ModeScalingTargetMissing).shown(kube.KindHPA),
	// A condition reason only: the matching event is FailedRescale.
	reason(reasons.FailedUpdateScale, ModeScalingUpdateFailed),
	reason(reasons.HPAInvalidSelector, ModeScalingInvalidSelector),
	reason(reasons.HPAMaxedOut, ModeScalingMaxedOut).causeOnly(),
	reason(reasons.HPAScalingError, ModeScalingError),
	// A scale target that does not exist and a Ready node whose kubelet
	// kwatch cannot reach are worth a look; whatever fails because of
	// them is the incident.
	reason(reasons.HPATargetMissing, ModeScalingTargetMissing).digest(),
	reason(reasons.WorkloadNeverReady, ModeNeverReady),
	reason(reasons.ReadinessFlapping, ModeReadinessFlapping),
	reason(reasons.KubeletUnreachable, ModeKubeletUnreachable).digest(),
	reason(reasons.KwatchNetworkRestricted, ModeKwatchNetwork),
	reason(reasons.JobRunningLong, ModeJobRunningLong),
	reason(reasons.ScaledToZeroRouted, ModeScaledToZero),
	reason(reasons.JobFailed, ModeJobFailed),
	reason(reasons.JobDeadlineExceeded, ModeJobFailedDeadline),
	reason(reasons.JobBackoffLimitExceeded, ModeJobFailedBackoffLimit),
	reason(reasons.JobSuspended, ModeSuspended),
	reason(reasons.CronJobSuspended, ModeSuspended),
	reason(reasons.CronJobNotScheduled, ModeNotScheduling),
	reason(reasons.CronJobInvalidSchedule, ModeInvalidSchedule),
	reason(reasons.PdbViolation, ModeDisruptionBudget),
	reason(reasons.ImageDigestDrift, ModeImageDrift),
	reason(reasons.ReleaseRegression, ModeReleaseRegression),
	reason(reasons.LeaseStale, ModeLeaseStale),
	reason(reasons.DeprecatedAPIInUse, ModeDeprecatedAPI),
	reason(reasons.APIServerErrors, ModeAPIServerErrors),
	reason(reasons.CoreDNSServfail, ModeDNSServfail),
	reason(reasons.NodePLEGSlow, ModePLEGSlow),
	// Evictions are the kubelet doing its job; the evicted pods are
	// the incidents. Idle objects are clutter, not failures.
	reason(reasons.NodeEvicting, ModeEvicting).digest(),
	reason(reasons.NodeConditionPrefix, ModeNodeCondition),
	reason(reasons.ServiceUnused, ModeUnused).digest(),
	reason(reasons.ClaimUnused, ModeUnused).digest(),
	reason(reasons.RiskPrefix, ModeRisk),
	reason(reasons.RiskNoReadinessProbe, ModeRiskNoReadinessProbe),
	reason(reasons.RiskNoMemoryLimit, ModeRiskNoMemoryLimit),
	reason(reasons.RiskMutableImageTag, ModeRiskMutableImageTag),
	reason(reasons.RiskSingleReplica, ModeRiskSingleReplica),
	reason(reasons.StatefulSetRolloutStuck, ModeStatefulSetRolloutStuck),
	reason(reasons.CronJobMissedRuns, ModeScheduleMissed),
	reason(reasons.CronJobBlocked, ModeScheduleBlocked),
	reason(reasons.CronJobRepeatedFailure, ModeScheduleRepeatedFailure),
	reason(reasons.CronJobLastRunFailed, ModeScheduleLastRunFailed),
	reason(reasons.CronJobNoRecentSuccess, ModeScheduleNoRecentSuccess),
	reason(reasons.LoadBalancerSyncFailed, ModeLoadBalancerSyncFailed),
	reason(reasons.IngressTLSSecretMissing, ModeIngressTLSSecretMissing),
	reason(reasons.IngressClassMissing, ModeIngressClassMissing),
	reason(reasons.PriorityClassMissing, ModeReferencePriorityClassMissing),
	reason(reasons.RuntimeClassMissing, ModeReferenceRuntimeClassMissing),
	// A budget selecting no pods protects nothing, but on a cluster that
	// scales workloads to zero it is the normal night.
	reason(reasons.PdbSelectsNothing, ModeBudgetSelectsNothing).digest(),
	reason(reasons.PdbOverlap, ModeBudgetOverlap),
	reason(reasons.PdbSyncFailed, ModeBudgetSyncFailed),
	reason(reasons.PdbBlocksDrain, ModeBudgetBlocksDrain),
	reason(reasons.ResourceClaimUnallocated, ModeDeviceUnallocated),
	reason(reasons.FailedPrepareDynamicResources, ModeDevicePrepareFailed).
		event(Critical),
	reason(reasons.VolumeDetachFailure, ModeVolumeDetachFailed),
	reason(reasons.VolumeAttachWaiting, ModeVolumeAttachWaiting),
	reason(reasons.VolumeMultiAttach, ModeVolumeMultiAttach),
	reason(reasons.ProvisioningFailed, ModeVolumeProvisioningFailed).
		event(Warning),
	reason(reasons.FailedMapVolume, ModeVolumeMapFailed).event(Critical),
	reason(reasons.CertificateDenied, ModeCertificateDenied),
	reason(reasons.CertificateNotIssued, ModeCertificateNotIssued),
	reason(reasons.CRDNotEstablished, ModeCRDNotEstablished),
}
