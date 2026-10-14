package main

// Everything the generator reads is named here. Nothing is found by a
// broad search: a reason is either emitted by a call the scanner can
// resolve (see calls.go) or is named below with the file that defines it,
// so a reviewer can see why each one is in the corpus.

// scanDirs are parsed and scanned for event calls. constDirs are parsed
// only so names in them can be resolved.
var scanDirs = []string{
	"pkg/kubelet", "pkg/scheduler", "pkg/controller", "pkg/volume",
	"staging/src/k8s.io/cloud-provider/controllers",
}

var constDirs = []string{
	"pkg/util/node", "pkg/registry/core/pod/storage",
	"staging/src/k8s.io/api/core/v1",
	"staging/src/k8s.io/api/batch/v1",
	"staging/src/k8s.io/api/certificates/v1",
	"staging/src/k8s.io/cri-api/pkg/errors",
}

// sparsePaths are checked out when the tag is cloned.
var sparsePaths = []string{
	"pkg", "staging/src/k8s.io/api", "staging/src/k8s.io/cri-api",
	"staging/src/k8s.io/cloud-provider",
}

// A ref is "dir:Name": the string constant or errors.New variable Name
// declared at package level in dir. A lit is a string literal that must
// appear in a file, for the reasons the source writes inline.
type lit struct{ file, value string }

type group struct {
	refs []string
	lits []lit
}

const (
	kubelet = "pkg/kubelet"
	cri     = "pkg/kubelet/kuberuntime"
	images  = "pkg/kubelet/images"
	criErr  = "staging/src/k8s.io/cri-api/pkg/errors"
	apiCore = "staging/src/k8s.io/api/core/v1"
	apiJob  = "staging/src/k8s.io/api/batch/v1"
	life    = "pkg/kubelet/lifecycle"
	plugins = "pkg/scheduler/framework/plugins/names"
)

// waiting: the reasons of a container in the waiting state. The pull
// errors, the run and create errors of the kubelet's sync results, and
// the two placeholders of kubelet_pods.go, and the CRI errors the
// image manager and kuberuntime pass through. The pod-level sync errors of
// sync_result.go (KillContainerError, CreatePodSandboxError, ...) never
// become a container waiting reason and are left out on purpose.
var waiting = group{refs: []string{
	kubelet + ":ContainerCreating", kubelet + ":PodInitializing",
	"pkg/kubelet/container:ErrCrashLoopBackOff",
	"pkg/kubelet/container:ErrRunContainer",
	images + ":ErrImagePullBackOff", images + ":ErrImageInspect",
	images + ":ErrImagePull", images + ":ErrImageNeverPull",
	images + ":ErrInvalidImageName",
	cri + ":ErrCreateContainerConfig", cri + ":ErrPreCreateHook",
	cri + ":ErrCreateContainer", cri + ":ErrPreStartHook",
	cri + ":ErrPostStartHook",
	// Passed through from the container runtime's CRI errors.
	criErr + ":ErrRegistryUnavailable",
	criErr + ":ErrSignatureValidationFailed",
	criErr + ":ErrRROUnsupported", criErr + ":ErrImageVolumeMountFailed",
}}

// terminated: what the kubelet itself writes or tests for. The rest
// (Completed, Error, StartError) is reported by the container runtime
// and is listed in upstream/states.go.
var terminated = group{refs: []string{
	"pkg/kubelet/container:ContainerReasonStatusUnknown",
}, lits: []lit{
	{"pkg/kubelet/kuberuntime/helpers.go", "OOMKilled"},
	{"pkg/kubelet/kuberuntime/kuberuntime_container.go",
		"ContainerCannotRun"},
	{"pkg/kubelet/kubelet_pods.go", "Completed"},
}}

// admission: the reasons a pod is rejected with at admission. The
// kubelet records each as a Warning event (rejectPod) and as the
// pod's status reason. They are the lifecycle handlers' reasons, the
// sysctl and topology admit handlers', the node shutdown manager's, and
// the scheduler plugin names the predicate admit handler returns as
// the reason of a failed predicate.
var admission = group{refs: []string{
	"pkg/kubelet/nodeshutdown:NodeShutdownNotAdmittedReason",
	"pkg/kubelet/sysctl:ForbiddenReason",
	"pkg/kubelet/cm/topologymanager:ErrorTopologyAffinity",
	life + ":UnexpectedAdmissionError", life + ":UnknownReason",
	life + ":UnexpectedPredicateFailureType",
	life + ":OutOfCPU", life + ":OutOfMemory",
	life + ":OutOfEphemeralStorage", life + ":OutOfPods",
	life + ":PodOSSelectorNodeLabelDoesNotMatch",
	life + ":PodOSNotSupported", life + ":InvalidNodeInfo",
	life + ":InitContainerRestartPolicyForbidden",
	life + ":SupplementalGroupsPolicyNotSupported",
	life + ":AppArmorNotAdmittedReason",
	life + ":PodLevelResourcesNotAdmittedReason",
	life + ":PodFeatureUnsupported",
	plugins + ":NodeAffinity", plugins + ":NodeName",
	plugins + ":NodePorts", plugins + ":TaintToleration",
}}

// pod: values of pod.status.reason. The admission reasons plus the
// kubelet's eviction, preemption, active deadline and shutdown paths
// and the node lifecycle controller.
var pod = group{refs: append([]string{
	"pkg/kubelet/eviction:Reason", "pkg/util/node:NodeUnreachablePodReason",
	"pkg/kubelet/events:PreemptContainer",
	kubelet + ":reason",
	"pkg/kubelet/nodeshutdown:nodeShutdownReason",
}, admission.refs...)}

// podCondition: reasons on a pod condition. The constants of
// k8s.io/api, the kubelet status manager's and the reasons the
// controllers and the eviction API write inline.
var podCondition = group{refs: []string{
	apiCore + ":PodReasonUnschedulable",
	apiCore + ":PodReasonSchedulingGated",
	apiCore + ":PodReasonSchedulerError",
	apiCore + ":PodReasonTerminationByKubelet",
	apiCore + ":PodReasonPreemptionByScheduler",
	apiCore + ":PodReasonDeferred", apiCore + ":PodReasonInfeasible",
	apiCore + ":PodReasonError",
	"pkg/kubelet/status:PodCompleted", "pkg/kubelet/status:PodFailed",
	"pkg/kubelet/status:ContainersNotReady",
	"pkg/kubelet/status:ContainersNotInitialized",
	"pkg/kubelet/status:UnknownContainerStatuses",
	"pkg/kubelet/status:ReadinessGatesNotReady",
	"pkg/kubelet/status:RestartAllContainersStarted",
}, lits: []lit{
	{"pkg/controller/podgc/gc_controller.go", "DeletionByPodGC"},
	{"pkg/controller/tainteviction/taint_eviction.go",
		"DeletionByTaintManager"},
	{"pkg/controller/devicetainteviction/device_taint_eviction.go",
		"DeletionByDeviceTaintManager"},
	{"pkg/registry/core/pod/storage/eviction.go",
		"EvictionByEvictionAPI"},
}}

// dynamic lists the event call sites whose reason is not a constant at
// the call, by file, with where the reasons come from. A call site in
// any other file that cannot be resolved fails the generator.
type dynamic struct {
	why   string
	refs  []string
	lits  []lit
	extra []string // reasons composed at run time, with evidence below
	warn  bool
}

var dynamicEvents = map[string][]dynamic{
	"pkg/kubelet/kubelet.go": {{
		why: "rejectPod records the admission failure reason of the " +
			"allocation manager as a Warning, and it is also the " +
			"pod's status reason",
		refs: admission.refs, warn: true,
	}},
	"pkg/controller/job/job_controller.go": {{
		why: "finishedCond.Reason of a failed Job",
		refs: []string{apiJob + ":JobReasonBackoffLimitExceeded",
			apiJob + ":JobReasonDeadlineExceeded",
			apiJob + ":JobReasonPodFailurePolicy",
			apiJob + ":JobReasonMaxFailedIndexesExceeded",
			apiJob + ":JobReasonFailedIndexes"},
		warn: true,
	}},
	"pkg/controller/statefulset/stateful_pod_control.go": {{
		why: "Successful<Verb> and Failed<Verb> with the verbs " +
			"create, update and delete (recordPodEvent, recordClaimEvent)",
		extra: []string{"SuccessfulCreate", "SuccessfulUpdate",
			"SuccessfulDelete"},
	}, {
		why: "the Failed<Verb> half of the same two functions",
		extra: []string{"FailedCreate", "FailedUpdate",
			"FailedDelete"},
		warn: true,
	}},
	"pkg/controller/volume/persistentvolume/pv_controller.go": {{
		why: "reason chosen between two constants of the volume " +
			"events package",
		refs: []string{
			"pkg/controller/volume/events:WaitForFirstConsumer",
			"pkg/controller/volume/events:WaitForPodScheduled"},
	}, {
		why: "RecyclerPod relays the events of the recycler pod with " +
			"their own type, so it is also a Warning",
		refs: []string{"pkg/controller/volume/events:RecyclerPod"},
		warn: true,
	}},
	"pkg/controller/volume/selinuxwarning/" +
		"selinux_warning_controller.go": {{
		why: "conflict.EventReason, set inline in cache/volumecache.go",
		lits: []lit{
			{"pkg/controller/volume/selinuxwarning/cache/" +
				"volumecache.go", "SELinuxChangePolicyConflict"},
			{"pkg/controller/volume/selinuxwarning/cache/" +
				"volumecache.go", "SELinuxLabelConflict"},
		},
	}},
	"pkg/kubelet/allocation/allocation_manager.go": {{
		why: "eventType holds one of two constants of kubelet/events",
		refs: []string{"pkg/kubelet/events:ResizeDeferred",
			"pkg/kubelet/events:ResizeInfeasible"},
		warn: true,
	}},
}

// extraEvidence is checked for the composed reasons above: each file
// must still contain these literals.
var extraEvidence = map[string][]string{
	"pkg/controller/statefulset/stateful_pod_control.go": {
		"Successful%s", "Failed%s", "create", "update", "delete"},
}

// conditionCall names a function that sets a condition, and which of its
// arguments is the reason.
type conditionCall struct {
	dir, fn string
	arg     int
}

// hpaConditionCalls: the HPA controller sets the reason of AbleToScale
// and ScalingActive with these two functions, so the reasons of
// kwatch's hpaFailures table can be checked against them.
var hpaConditionCalls = []conditionCall{
	{"pkg/controller/podautoscaler", "setCondition", 3},
	{"pkg/controller/podautoscaler",
		"getUnableComputeReplicaCountCondition", 1},
}
