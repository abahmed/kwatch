package upstream

// WaitingReasons are the reasons of a container in the waiting state.
// Sources: pkg/kubelet/kuberuntime/kuberuntime_container.go and
// kuberuntime_manager.go (start errors), pkg/kubelet/images/types.go
// (image pull errors), pkg/kubelet/container/sync_result.go
// (ErrCreateContainerConfig and friends) and pkg/kubelet/kubelet_pods.go
// (ContainerCreating, PodInitializing).
var WaitingReasons = []string{
	"ContainerCreating",
	"PodInitializing",
	"CrashLoopBackOff",
	"ImagePullBackOff",
	"ErrImagePull",
	"ErrImageNeverPull",
	"ImageInspectError",
	"InvalidImageName",
	"RegistryUnavailable",
	"CreateContainerConfigError",
	"CreateContainerError",
	"RunContainerError",
	"PreStartHookError",
	"PostStartHookError",
	"PreCreateHookError",
}

// TerminatedReasons are the reasons of a container in the terminated
// state. Sources: the container runtimes' CRI status (containerd and
// CRI-O report Completed, Error, OOMKilled, ContainerCannotRun,
// StartError) and pkg/kubelet/kubelet_pods.go (ContainerStatusUnknown,
// DeadlineExceeded for activeDeadlineSeconds).
var TerminatedReasons = []string{
	"Completed",
	"Error",
	"OOMKilled",
	"ContainerCannotRun",
	"StartError",
	"DeadlineExceeded",
	"ContainerStatusUnknown",
}

// PodPhases are the values of pod.status.phase (k8s.io/api core/v1
// PodPhase constants).
var PodPhases = []string{
	"Pending", "Running", "Succeeded", "Failed", "Unknown",
}

// PodReasons are the values of pod.status.reason the kubelet, the
// scheduler and the node lifecycle controller write. Sources:
// pkg/kubelet/eviction (Evicted), pkg/kubelet/lifecycle/predicate.go
// (admission rejections; OutOf<resource> stands for OutOfcpu,
// OutOfmemory, OutOfpods and OutOfephemeral-storage),
// pkg/kubelet/nodeshutdown (Terminated, NodeShutdown),
// pkg/kubelet/preemption, pkg/kubelet/kubelet.go (DeadlineExceeded) and
// pkg/controller/nodelifecycle (NodeLost).
var PodReasons = []string{
	"Evicted",
	"NodeLost",
	"Preempting",
	"UnexpectedAdmissionError",
	"UnexpectedPredicateFailureType",
	"OutOfcpu",
	"OutOfmemory",
	"OutOfpods",
	"OutOfephemeral-storage",
	"NodeAffinity",
	"NodeName",
	"NodePorts",
	"TaintToleration",
	"PodOSNotSupported",
	"PodOSSelectorNodeLabelDoesNotMatch",
	"TopologyAffinityError",
	"SysctlForbidden",
	"InvalidNodeInfo",
	"NodeShutdown",
	"Terminated",
	"DeadlineExceeded",
}

// PodConditionReasons are reasons Kubernetes puts on a pod condition.
// Sources: k8s.io/api core/v1 (PodReasonUnschedulable,
// PodReasonSchedulingGated, PodReasonSchedulerError,
// PodReasonPreemptionByScheduler) and pkg/kubelet/status
// (TerminationByKubelet, DeletionByTaintManager, EvictionByEvictionAPI,
// DeletionByPodGC).
var PodConditionReasons = []string{
	"Unschedulable",
	"SchedulingGated",
	"SchedulerError",
	"PreemptionByScheduler",
	"TerminationByKubelet",
	"DeletionByTaintManager",
	"EvictionByEvictionAPI",
	"DeletionByPodGC",
	"ContainersNotReady",
	"ContainersNotInitialized",
	"PodCompleted",
}
