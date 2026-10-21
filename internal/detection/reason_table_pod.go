package detection

import (
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// podReasons is the pod, container and scheduling reasons.
var podReasons = []ReasonInfo{
	reason(reasons.Completed, ModeCompleted),
	reason(reasons.Error, ModeError),
	reason(reasons.ErrImagePull, ModeImagePull).waiting(Critical).
		shown(podAndContainer...),
	reason(reasons.ImagePullBackOff, ModeImagePull).waiting(Critical).
		shown(podAndContainer...),
	reason(reasons.ImageInspectError, ModeImagePullInspect).waiting(Critical),
	reason(reasons.InvalidImageName, ModeImagePullInvalidName).waiting(Critical),
	reason(reasons.CrashLoopBackOff, ModeCrashLoop).waiting(Critical),
	reason(reasons.OOMKilled, ModeOOMKilled),
	reason(reasons.OOMKILLED, ModeOOMKilled),
	reason(reasons.HighRestartCount, ModeRestarting),
	reason(reasons.ContainerCreating, ModeCreating).progress(),
	reason(reasons.PodInitializing, ModeInitializing).progress(),
	reason(reasons.PodCompleted, ModeCompleted),
	reason(reasons.ContainersNotReady, ModeNotReady),
	reason(reasons.CreateContainerError, ModeCreateError).waiting(Critical),
	reason(reasons.CreateConfigError, ModeCreateErrorConfig).waiting(Critical),
	reason(reasons.InitContainerError, ModeInitError),
	reason(reasons.InitContainerWaiting, ModeInitWaiting),
	reason(reasons.DeadlineExceeded, ModeDeadlineExceeded).
		shown(kube.KindPod, kube.KindJob),
	reason(reasons.PostStartHookError, ModeHookPostStart).waiting(Critical),
	reason(reasons.StartupProbeFailed, ModeProbeStartup),
	reason(reasons.Unschedulable, ModeUnschedulable),
	reason(reasons.PodPending, ModePending),
	reason(reasons.PodFailed, ModeFailed),
	reason(reasons.PodStatusUnknown, ModeStatusUnknown),
	reason(reasons.PodStuckTerminating, ModeStuckDeleting).digest(),
	reason(reasons.SchedulingGated, ModeSchedulingGated),
	reason(reasons.FailedScheduling, ModeUnschedulable).
		shown(kube.KindPod).
		label("can't be scheduled"),
	reason(reasons.Evicted, ModeEvicted).shown(kube.KindPod),
	reason(reasons.RegistryUnavailable, ModeImagePullRegistry).
		waiting(Critical),
	reason(reasons.LivenessProbeFailed, ModeProbeLiveness),
	reason(reasons.LivenessKilled, ModeCrashLoopLiveness),
	reason(reasons.ReadinessProbeFailed, ModeProbeReadiness),
	reason(reasons.PodAdmissionRejected, ModeAdmissionRejected),
	reason(reasons.ExitNotExecutable, ModeExitNotExecutable),
	reason(reasons.ExitCommandNotFound, ModeExitCommandNotFound),
	reason(reasons.ExitKilled, ModeExitKilled),
	reason(reasons.ExitSegfault, ModeExitSegfault),
	reason(reasons.ExitIllegalInstruction, ModeExitIllegal),
	reason(reasons.ExitAborted, ModeExitAborted),
	reason(reasons.ExitBusError, ModeExitBusError),
	reason(reasons.ExitFloatingPoint, ModeExitFloatingPoint),
	reason(reasons.ErrImageNeverPull, ModeImagePullNeverPull).waiting(Critical),
	reason(reasons.PreStartHookError, ModeHookPreStart).waiting(Critical),
	reason(reasons.EvictionThresholdMet, ModeDiskEvictionThreshold).event(Warning),
	reason(reasons.ImageGCFailed, ModeDiskImageGCFailed).event(Info),
	reason(reasons.FreeDiskSpaceFailed, ModeDiskFreeSpaceFailed).event(Info),
	reason(reasons.ContainerGCFailed, ModeDiskContainerGCFailed).event(Warning),
	reason(reasons.NodePodIPExhausted, ModeNetworkIPExhausted),
	reason(reasons.NodeCNINotReady, ModeNetworkCNINotReady),
	reason(reasons.NodeHeartbeatStale, ModeHeartbeatStale),
	reason(reasons.PodResizeInfeasible, ModeResizeInfeasible),
	reason(reasons.PodResizeDeferred, ModeResizeDeferred),
	reason(reasons.PodResizeError, ModeResizeError),
	reason(reasons.PodPreemptedRepeatedly, ModePreemptedRepeatedly),
	reason(reasons.PodPreempted, ModePreempted),
	// A hard kill at the end of the grace period is a shutdown that
	// did not finish; the workload failing after it is the incident.
	reason(reasons.PodKilledAtGrace, ModeKilledGracePeriod).digest(),
}
